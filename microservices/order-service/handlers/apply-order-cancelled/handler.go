// Package applyordercancelled handles the OrderCancelled events of
// orders.events: the order becomes CANCELLED, with the engine's reason when
// there is one (no_liquidity for a market order's remainder).
package applyordercancelled

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A store failure is returned so the consumer retries the event; a payload
// that cannot be read is logged and skipped.
func Handle(ctx *gofr.Context, message events.Message) error {
	var cancelled events.OrderCancelled
	if err := message.ParsePayload(&cancelled); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	var reason *string
	if cancelled.Reason != "" {
		reason = &cancelled.Reason
	}

	return orderdb.UpdateCancelledOrder(ctx, cancelled.OrderID, reason, message.CreatedAt)
}
