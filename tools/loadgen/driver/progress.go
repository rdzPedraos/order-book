package driver

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// Accepted orders, and first engine events read: processed also counts the
// rare event of an order someone else sent during the run.
type counts struct {
	accepted  int
	processed int
}

// Writes a line every interval until stop is called, so a backlog shows while
// it grows and while it drains.
func startProgress(ctx context.Context, writer io.Writer, every time.Duration, tracker *Tracker) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)

	var done sync.WaitGroup
	done.Go(func() { writeProgress(ctx, writer, every, tracker) })

	return func() {
		cancel()
		done.Wait()
	}
}

func writeProgress(ctx context.Context, writer io.Writer, every time.Duration, tracker *Tracker) {
	startedAt := time.Now()
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	var last counts

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		current := tracker.countOrders()
		fmt.Fprintln(writer, formatProgress(time.Since(startedAt), every, current, last))
		last = current
	}
}

func formatProgress(elapsed, every time.Duration, current, last counts) string {
	return fmt.Sprintf("%5s  API %6.0f/s  engine %6.0f/s  backlog %d", elapsed.Round(time.Second),
		float64(current.accepted-last.accepted)/every.Seconds(), float64(current.processed-last.processed)/every.Seconds(),
		max(current.accepted-current.processed, 0))
}
