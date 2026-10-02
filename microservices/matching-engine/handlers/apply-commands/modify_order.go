package applycommands

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

// Recomputes what the order must have reserved at its new price and pending
// quantity. More is reserved on the spot, before the change, so the order
// stays unchanged if the funds are short; less is released with the batch. A
// modification that cannot apply changes nothing, so it is only logged.
func applyModifyOrder(ctx *gofr.Context, current *batch, applied *command) error {
	var modify events.ModifyOrder
	if err := applied.record.Message.ParsePayload(&modify); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", applied.record.Message.ID, err)

		return nil
	}

	applied.sequence = applied.book.NextSequence()

	order, reason := getModifiableOrder(applied, modify)
	if reason == "" {
		var err error

		reason, err = changeOrder(ctx, current, applied, order, modify)
		if err != nil {
			return err
		}
	}

	if reason != "" {
		ctx.Logger.Infof("ignoring modification of order %s: %s", modify.OrderID, reason)
	}

	return nil
}

// The reason is empty when the order can be modified.
func getModifiableOrder(applied *command, modify events.ModifyOrder) (*models.Order, string) {
	order, inBook := applied.book.Get(modify.OrderID)

	switch {
	case inBook && order.UserID != modify.UserID:
		return nil, reasonNotOwner
	case inBook:
		return order, ""
	case applied.book.IsKnownOrder(modify.OrderID):
		return nil, reasonFinalOrder
	default:
		return nil, reasonUnknownOrder
	}
}

// Answers the rejection reason, empty when the order changed.
func changeOrder(ctx *gofr.Context, current *batch, applied *command, order *models.Order, modify events.ModifyOrder) (string, error) {
	price, quantity := getValueOr(modify.Limit, order.Price), getValueOr(modify.Quantity, order.Quantity)

	currency, required, err := getRequiredReservation(applied.record.Message.Book, order.Side, quantity, price)
	if err != nil {
		return events.ReasonInvalidAmount, nil
	}

	if required > order.Reserved {
		reserved, err := reserveDifference(ctx, applied, order, currency, required-order.Reserved)
		if err != nil || !reserved {
			return events.ReasonInsufficientFunds, err
		}
	}

	if required < order.Reserved {
		current.release(applied.record.Message.ID, *order, currency, order.Reserved-required)
	}

	moveInBook(applied, order, price, quantity)
	order.Reserved = required

	current.emit(ctx, applied, events.RouteOrderModified, order.ID, order.UserID, func(header events.EventHeader) any {
		return events.OrderModified{EventHeader: header, Limit: &price, Quantity: &quantity}
	})

	return "", nil
}

// Only lowering the quantity at the same price keeps the order's place;
// any other change sends it to the end of its new level.
func moveInBook(applied *command, order *models.Order, price, quantity int64) {
	if price == order.Price && quantity <= order.Quantity {
		applied.book.ReduceQuantity(order.ID, quantity)

		return
	}

	applied.book.Remove(order.ID)
	order.Price, order.Quantity, order.Sequence = price, quantity, applied.sequence
	applied.book.Put(order)
}

func getRequiredReservation(bookID string, side models.Side, quantity, price int64) (money.Currency, int64, error) {
	book, err := books.Normalize(bookID)
	if err != nil {
		return "", 0, err
	}

	if side == models.SideSell {
		return book.Base, quantity, nil
	}

	notional, err := money.Notional(book.Base, quantity, price)

	return book.Quote, notional, err
}

func reserveDifference(ctx *gofr.Context, applied *command, order *models.Order, currency money.Currency, amount int64) (bool, error) {
	results, err := applyFunds(ctx, []walletclient.Operation{{
		Type: walletclient.TypeReserve, MessageID: applied.record.Message.ID, OrderID: order.ID,
		UserID: order.UserID, Currency: currency, Amount: amount,
	}})
	if err != nil {
		return false, err
	}

	return results[0].Result == walletclient.ResultOK, nil
}

func getValueOr(value *int64, fallback int64) int64 {
	if value == nil {
		return fallback
	}

	return *value
}
