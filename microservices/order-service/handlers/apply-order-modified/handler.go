// Package applyordermodified handles the OrderModified events of
// orders.events: the order takes its new limit and quantity.
package applyordermodified

import (
	"errors"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

var errMissingTerms = errors.New("the event has no limit or quantity")

// A store failure is returned so the consumer retries the event; a payload
// that cannot be read is logged and skipped.
func Handle(ctx *gofr.Context, message events.Message) error {
	var modified events.OrderModified
	if err := parseModified(message, &modified); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	return orderdb.UpdateModifiedOrder(ctx, modified.OrderID, *modified.Limit, *modified.Quantity, message.CreatedAt)
}

// The engine always sends both: the new limit and what is still pending.
func parseModified(message events.Message, modified *events.OrderModified) error {
	if err := message.ParsePayload(modified); err != nil {
		return err
	}

	if modified.Limit == nil || modified.Quantity == nil {
		return errMissingTerms
	}

	return nil
}
