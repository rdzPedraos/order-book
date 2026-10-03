package applycommands

import (
	"time"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
)

const publishRetryDelay = 200 * time.Millisecond

// The batch's events go to the log together, so the batch waits for the log
// once. What the log did not confirm is retried, from the first event it did
// not confirm, until it does: no event is lost, reordered or written twice
// while the engine runs.
func publishEvents(ctx *gofr.Context, messages []events.Message) error {
	for len(messages) > 0 {
		published, err := producer.PublishBatch(ctx, messages)
		messages = messages[published:]

		if err == nil {
			continue
		}

		ctx.Logger.Errorf("publishing %d events, retrying: %v", len(messages), err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(publishRetryDelay):
		}
	}

	return nil
}
