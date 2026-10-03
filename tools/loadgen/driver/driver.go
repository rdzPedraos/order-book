// Package driver runs one load against the API: it funds the people of the
// run, sends the requests of a workload at the rate of a pace schedule with
// a pool of workers, and pairs each created order with the engine's first
// event about it, read from orders.events by a Tracker.
package driver

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rdzpedraos/order-book/shared/money"
	"github.com/rdzpedraos/order-book/tools/loadgen/pace"
	"github.com/rdzpedraos/order-book/tools/loadgen/stats"
	"github.com/rdzpedraos/order-book/tools/loadgen/workload"
)

// Enough for any order of the run, so no order is rejected for its funds.
const (
	fundedBRL = "100000000.00"
	fundedVIB = "1000000"
)

// EventWait is how long the run waits, after sending, for the last engine
// events. Progress gets the phases of the run and a line every ProgressEvery.
type Config struct {
	OrdersURL     string
	WalletURL     string
	Book          string
	People        []string
	Workers       int
	Duration      time.Duration
	EventWait     time.Duration
	Schedule      pace.Schedule
	Generator     *workload.Generator
	Progress      io.Writer
	ProgressEvery time.Duration
}

// Acceptance is the latency until the API answered; Engine until the
// engine's first event about the order. MissingEvents are accepted orders
// whose event never arrived within EventWait. EngineRate is how many orders
// per second the engine processed.
type Report struct {
	Sent          int
	Accepted      int
	Failed        int
	Elapsed       time.Duration
	Acceptance    stats.Summary
	Engine        stats.Summary
	MissingEvents int
	EngineRate    float64
}

func Run(ctx context.Context, config Config, tracker *Tracker) (*Report, error) {
	api, err := newClient(config)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(config.Progress, "funding %d people\n", len(config.People))
	if err := fundPeople(ctx, api, config.People); err != nil {
		return nil, err
	}

	fmt.Fprintf(config.Progress, "sending for %s\n", config.Duration)
	stopProgress := startProgress(ctx, config.Progress, config.ProgressEvery, tracker)
	sendStartedAt := time.Now()
	report, acceptance := sendRequests(ctx, api, config, tracker)

	fmt.Fprintf(config.Progress, "waiting up to %s for the last engine events\n", config.EventWait)
	waitForEvents(ctx, tracker, config.EventWait)
	stopProgress()

	latencies, missing := tracker.getLatencies()
	report.Acceptance, report.Engine, report.MissingEvents = stats.Summarize(acceptance), stats.Summarize(latencies), missing
	report.EngineRate = tracker.getEngineRate(sendStartedAt)

	return report, nil
}

// A backlog shows as seconds until the event that grow with the run, so an
// engine that keeps up answers every order, and 99 % of them within a second.
func (r Report) IsEngineKeepingUp() bool {
	return r.Engine.Count > 0 && r.MissingEvents == 0 && r.Engine.P99 < time.Second
}

func fundPeople(ctx context.Context, api *client, people []string) error {
	for _, userID := range people {
		if err := api.postDeposit(ctx, userID, money.BRL, fundedBRL); err != nil {
			return fmt.Errorf("fund %s: %w", userID, err)
		}

		if err := api.postDeposit(ctx, userID, money.VIB, fundedVIB); err != nil {
			return fmt.Errorf("fund %s: %w", userID, err)
		}
	}

	return nil
}

type outcome struct {
	latency time.Duration
	failed  bool
}

func sendRequests(ctx context.Context, api *client, config Config, tracker *Tracker) (*Report, []time.Duration) {
	requests := make(chan workload.Request, config.Workers)
	outcomes := make(chan outcome, config.Workers)
	startedAt := time.Now()

	var workers sync.WaitGroup
	for range config.Workers {
		workers.Go(func() { sendEach(ctx, api, tracker, requests, outcomes) })
	}

	go func() {
		pushDue(ctx, config, requests, startedAt)
		close(requests)
		workers.Wait()
		close(outcomes)
	}()

	report := &Report{}
	acceptance := make([]time.Duration, 0)

	for result := range outcomes {
		report.Sent++
		if result.failed {
			report.Failed++

			continue
		}

		report.Accepted++
		acceptance = append(acceptance, result.latency)
	}

	report.Elapsed = time.Since(startedAt)

	return report, acceptance
}

// Every millisecond it hands the workers what the schedule says is due, so
// a slow request delays no other.
func pushDue(ctx context.Context, config Config, requests chan<- workload.Request, startedAt time.Time) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	pushed := 0

	for ctx.Err() == nil {
		elapsed := time.Since(startedAt)
		if elapsed >= config.Duration {
			return
		}

		for due := config.Schedule.CountDue(elapsed); pushed < due; pushed++ {
			requests <- config.Generator.BuildNext()
		}

		<-ticker.C
	}
}

func sendEach(ctx context.Context, api *client, tracker *Tracker, requests <-chan workload.Request, outcomes chan<- outcome) {
	for request := range requests {
		sentAt := time.Now()

		orderID, err := api.postOrder(ctx, request)
		if err != nil {
			outcomes <- outcome{failed: true}

			continue
		}

		tracker.AddSent(orderID, sentAt)
		outcomes <- outcome{latency: time.Since(sentAt)}
	}
}

func waitForEvents(ctx context.Context, tracker *Tracker, wait time.Duration) {
	deadline := time.Now().Add(wait)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) && ctx.Err() == nil {
		if _, missing := tracker.getLatencies(); missing == 0 {
			return
		}

		<-ticker.C
	}
}
