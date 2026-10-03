// Command loadgen answers whether the deployed stack keeps up with a rate of
// orders: it sends orders to the API at that rate, measures how long the API
// takes to accept each one and how long until the engine publishes its first
// event about it, and ends with a verdict (design D4 of
// 06-container-infrastructure). It runs inside the cluster, where it reaches
// the services and Redpanda by their names:
//
//	loadgen -rate 5000 -duration 10m
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/tools/loadgen/driver"
	"github.com/rdzpedraos/order-book/tools/loadgen/pace"
	"github.com/rdzpedraos/order-book/tools/loadgen/stats"
	"github.com/rdzpedraos/order-book/tools/loadgen/workload"
)

// Where loadgen reaches the stack from inside the cluster, and the people
// it funds and trades as.
const (
	ordersURL = "http://orderbook-order-api"
	walletURL = "http://orderbook-wallet-api"
	broker    = "redpanda:9093"
	bookID    = "BRL-VIB"
	people    = 5000
	workers   = 200
	progress  = 5 * time.Second
)

var errUnknownProfile = errors.New("unknown profile: use B (a fixed rate) or E (a burst)")

type flags struct {
	profile        string
	rate, peak     int
	duration, wait time.Duration
}

func main() {
	options := parseFlags()
	printHeader(options)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	report, err := run(ctx, options)
	if err != nil {
		log.Fatal(err)
	}

	printReport(report)
}

func parseFlags() flags {
	var options flags

	flag.StringVar(&options.profile, "profile", "B", "load profile: B at a fixed rate, or E in a burst")
	flag.IntVar(&options.rate, "rate", 1000, "orders per second (the base rate of E)")
	flag.IntVar(&options.peak, "peak", 10000, "orders per second at the peak of E")
	flag.DurationVar(&options.duration, "duration", time.Minute, "how long to send")
	flag.DurationVar(&options.wait, "wait", 30*time.Second, "how long to wait for the last engine events")
	flag.Parse()

	return options
}

func run(ctx context.Context, options flags) (*driver.Report, error) {
	book, err := books.Normalize(bookID)
	if err != nil {
		return nil, err
	}

	schedule, err := buildSchedule(options)
	if err != nil {
		return nil, err
	}

	tracker := driver.NewTracker(time.Now())

	err = consumer.StartPartition(ctx, []string{broker}, events.TopicOrderEvents, book.Partition, 1000, logger{}, tracker.ApplyRecords)
	if err != nil {
		return nil, err
	}

	return driver.Run(ctx, driver.Config{
		OrdersURL: ordersURL, WalletURL: walletURL, Book: book.ID, People: workload.ListPeople(people), Workers: workers,
		Duration: options.duration, EventWait: options.wait, Schedule: schedule, Generator: workload.NewGenerator(people, 1),
		Progress: os.Stdout, ProgressEvery: progress,
	}, tracker)
}

func buildSchedule(options flags) (pace.Schedule, error) {
	switch options.profile {
	case "B":
		return pace.NewConstant(options.rate), nil
	case "E":
		return pace.NewBurst(options.rate, options.peak, options.duration), nil
	default:
		return nil, errUnknownProfile
	}
}

func printHeader(options flags) {
	if options.profile == "E" {
		fmt.Printf("profile E, %d/s with a peak of %d/s, for %s\n", options.rate, options.peak, options.duration)

		return
	}

	fmt.Printf("profile %s, %d/s for %s\n", options.profile, options.rate, options.duration)
}

func printReport(report *driver.Report) {
	verdict := "falls behind"
	if report.IsEngineKeepingUp() {
		verdict = "keeps up"
	}

	fmt.Printf("sent %d, accepted %d, failed %d: %.0f accepted/s\n",
		report.Sent, report.Accepted, report.Failed, float64(report.Accepted)/report.Elapsed.Seconds())
	printSummary("API answer", report.Acceptance)
	printSummary("engine event", report.Engine)
	fmt.Printf("orders without their engine event: %d\n", report.MissingEvents)
	fmt.Printf("verdict: the engine %s, it processed %.0f orders/s\n", verdict, report.EngineRate)
}

func printSummary(name string, summary stats.Summary) {
	fmt.Printf("%-13s p50=%s p90=%s p99=%s p99.9=%s max=%s\n", name, summary.P50.Round(time.Millisecond),
		summary.P90.Round(time.Millisecond), summary.P99.Round(time.Millisecond),
		summary.P999.Round(time.Millisecond), summary.Max.Round(time.Millisecond))
}

type logger struct{}

func (logger) Errorf(format string, args ...any) {
	log.Printf(format, args...)
}
