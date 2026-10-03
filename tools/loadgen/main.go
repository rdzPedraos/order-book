// Command loadgen sends a load profile to the API at a fixed rate and
// reports how many orders it sent, how long the API took to accept them and
// how long until the engine published its first event about each one (design
// D4 of 06-container-infrastructure). It runs inside the cluster, where it
// reaches the services and Redpanda by their names:
//
//	loadgen -profile B -rate 5000 -duration 10m
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/tools/loadgen/driver"
	"github.com/rdzpedraos/order-book/tools/loadgen/pace"
	"github.com/rdzpedraos/order-book/tools/loadgen/stats"
	"github.com/rdzpedraos/order-book/tools/loadgen/workload"
)

type flags struct {
	orders, wallet, host, brokers, book, profile string
	rate, peak, people, workers                  int
	duration, wait                               time.Duration
	seed                                         uint64
}

func main() {
	options := parseFlags()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	report, err := run(ctx, options)
	if err != nil {
		log.Fatal(err)
	}

	printReport(options, report)
}

func parseFlags() flags {
	var options flags

	flag.StringVar(&options.orders, "orders", "http://orderbook-order-api", "OrderService base URL")
	flag.StringVar(&options.wallet, "wallet", "http://orderbook-wallet-api", "WalletService base URL")
	flag.StringVar(&options.host, "host", "", "Host header, to go through the ingress")
	flag.StringVar(&options.brokers, "brokers", "redpanda:9093", "Redpanda brokers, comma separated")
	flag.StringVar(&options.book, "book", "BRL-VIB", "book to trade")
	flag.StringVar(&options.profile, "profile", "B", "load profile: A, B, C, D or E")
	flag.IntVar(&options.rate, "rate", 1000, "requests per second (the base rate of E)")
	flag.IntVar(&options.peak, "peak", 10000, "requests per second at the peak of E")
	flag.IntVar(&options.people, "people", 200, "people trading, funded at the start")
	flag.IntVar(&options.workers, "workers", 200, "requests in flight at once")
	flag.DurationVar(&options.duration, "duration", time.Minute, "how long to send")
	flag.DurationVar(&options.wait, "wait", 30*time.Second, "how long to wait for the last engine events")
	flag.Uint64Var(&options.seed, "seed", 1, "seed of the requests")
	flag.Parse()

	return options
}

func run(ctx context.Context, options flags) (*driver.Report, error) {
	book, err := books.Normalize(options.book)
	if err != nil {
		return nil, err
	}

	generator, err := workload.NewGenerator(options.profile, options.people, options.seed)
	if err != nil {
		return nil, err
	}

	tracker := driver.NewTracker(time.Now())

	err = consumer.StartPartition(ctx, strings.Split(options.brokers, ","), events.TopicOrderEvents, book.Partition,
		1000, logger{}, tracker.ApplyRecords)
	if err != nil {
		return nil, err
	}

	return driver.Run(ctx, driver.Config{
		OrdersURL: options.orders, WalletURL: options.wallet, Host: options.host, Book: book.ID,
		People: workload.ListPeople(options.people), Workers: options.workers,
		Duration: options.duration, EventWait: options.wait,
		Schedule: buildSchedule(options), Generator: generator,
	}, tracker)
}

func buildSchedule(options flags) pace.Schedule {
	if options.profile == "E" {
		return pace.NewBurst(options.rate, options.peak, options.duration)
	}

	return pace.NewConstant(options.rate)
}

func printReport(options flags, report *driver.Report) {
	fmt.Printf("profile %s, %d/s for %s\n", options.profile, options.rate, options.duration)
	fmt.Printf("sent %d, accepted %d, failed %d, in %s: %.0f accepted/s\n",
		report.Sent, report.Accepted, report.Failed, report.Elapsed.Round(time.Millisecond),
		float64(report.Accepted)/report.Elapsed.Seconds())
	printSummary("API accepted", report.Acceptance)
	printSummary("engine event", report.Engine)
	fmt.Printf("orders without their engine event: %d\n", report.MissingEvents)
}

func printSummary(name string, summary stats.Summary) {
	fmt.Printf("%-13s n=%d p50=%s p90=%s p99=%s p99.9=%s max=%s\n", name, summary.Count,
		summary.P50.Round(time.Microsecond*100), summary.P90.Round(time.Microsecond*100),
		summary.P99.Round(time.Microsecond*100), summary.P999.Round(time.Microsecond*100), summary.Max.Round(time.Millisecond))
}

type logger struct{}

func (logger) Errorf(format string, args ...any) {
	log.Printf(format, args...)
}
