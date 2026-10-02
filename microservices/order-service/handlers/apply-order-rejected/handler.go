// Package applyorderrejected handles the OrderRejected events of
// orders.events: the order becomes REJECTED with the engine's reason. The
// event may arrive before the order's NewOrder, so it carries the order's
// details to store it.
package applyorderrejected

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A store failure is returned so the consumer retries the event; a payload
// that cannot be read is logged and skipped.
func Handle(ctx *gofr.Context, message events.Message) error {
	var rejected events.OrderRejected
	if err := message.ParsePayload(&rejected); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	order := models.NewOrderFromDetails(rejected.EventHeader, message.Book, rejected.OrderDetails, models.StatusRejected, message.CreatedAt)
	order.Reason = &rejected.Reason

	return orderdb.InsertOrUpdateOrder(ctx, order)
}
