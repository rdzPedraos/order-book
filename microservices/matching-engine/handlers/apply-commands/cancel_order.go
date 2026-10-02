package applycommands

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

const (
	reasonNotOwner     = "not the owner"
	reasonFinalOrder   = "order already final"
	reasonUnknownOrder = "unknown order"
)

// Takes the order out of the book and frees what it still has reserved. A
// cancellation that cannot apply changes nothing, so it is only logged. An
// order no longer in the book but known to the engine is already final.
func applyCancelOrder(ctx *gofr.Context, current *batch, applied *command) error {
	var cancel events.CancelOrder
	if err := applied.record.Message.ParsePayload(&cancel); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", applied.record.Message.ID, err)

		return nil
	}

	applied.sequence = applied.book.NextSequence()

	order, reason := getCancellableOrder(applied, cancel)
	if reason != "" {
		ctx.Logger.Infof("ignoring cancellation of order %s: %s", cancel.OrderID, reason)

		return nil
	}

	currency, err := getReservedCurrency(applied.record.Message.Book, order.Side)
	if err != nil {
		ctx.Logger.Errorf("cancelling order %s: %v", order.ID, err)

		return nil
	}

	applied.book.Remove(order.ID)
	current.release(applied.record.Message.ID, *order, currency, order.Reserved)

	current.emit(ctx, applied, events.RouteOrderCancelled, order.ID, order.UserID, func(header events.EventHeader) any {
		return events.OrderCancelled{EventHeader: header, CancelledQuantity: order.Quantity, Released: order.Reserved}
	})

	return nil
}

// The reason is empty when the order can be cancelled.
func getCancellableOrder(applied *command, cancel events.CancelOrder) (*models.Order, string) {
	order, inBook := applied.book.Get(cancel.OrderID)

	switch {
	case inBook && order.UserID != cancel.UserID:
		return nil, reasonNotOwner
	case inBook:
		return order, ""
	case applied.book.IsKnownOrder(cancel.OrderID):
		return nil, reasonFinalOrder
	default:
		return nil, reasonUnknownOrder
	}
}

// A buy reserves the quote and a sell the base.
func getReservedCurrency(bookID string, side models.Side) (money.Currency, error) {
	book, err := books.Normalize(bookID)
	if err != nil {
		return "", err
	}

	if side == models.SideSell {
		return book.Base, nil
	}

	return book.Quote, nil
}
