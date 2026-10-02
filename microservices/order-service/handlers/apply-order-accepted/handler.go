// Package applyorderaccepted handles the OrderAccepted events of orders.events:
// a limit order becomes OPEN, and a market order stays PENDING until its
// trades fill it or its remainder is cancelled. The event may arrive before
// the order's NewOrder, so it carries the order's details to store it.
package applyorderaccepted

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A store failure is returned so the consumer retries the event; a payload
// that cannot be read is logged and skipped.
func Handle(ctx *gofr.Context, message events.Message) error {
	var accepted events.OrderAccepted
	if err := message.ParsePayload(&accepted); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	status := models.StatusOpen
	if accepted.Type == string(models.TypeMarket) {
		status = models.StatusPending
	}

	order := models.NewOrderFromDetails(accepted.EventHeader, message.Book, accepted.OrderDetails, status, message.CreatedAt)

	return orderdb.InsertOrUpdateOrder(ctx, order)
}
