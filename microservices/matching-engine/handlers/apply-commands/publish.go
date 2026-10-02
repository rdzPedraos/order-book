package applycommands

import (
	"time"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
)

const publishRetryDelay = 200 * time.Millisecond

// Each event is retried until the log confirms it, in order, so no event of
// the batch is lost or published out of order while the engine runs.
func publishEvents(ctx *gofr.Context, messages []events.Message) error {
	for _, message := range messages {
		if err := publishWithRetry(ctx, message); err != nil {
			return err
		}
	}

	return nil
}

func publishWithRetry(ctx *gofr.Context, message events.Message) error {
	for {
		err := producer.Publish(ctx, message)
		if err == nil {
			return nil
		}

		ctx.Logger.Errorf("publishing %s %s, retrying: %v", message.Route, message.ID, err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(publishRetryDelay):
		}
	}
}
