package applycommands

import (
	"errors"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

var errMissingAmount = errors.New("the order has no amount to reserve")

// A new order with what it must reserve; err when that cannot be computed.
type preparedOrder struct {
	newOrder events.NewOrder
	currency money.Currency
	amount   int64
	err      error
}

func (p *preparedOrder) reservation(operationType string, messageID uuid.UUID) walletclient.Operation {
	return walletclient.Operation{
		Type: operationType, MessageID: messageID, OrderID: p.newOrder.OrderID,
		UserID: p.newOrder.UserID, Currency: p.currency, Amount: p.amount,
	}
}

// Nil for a record that is not a new order to apply: another command, an
// applied message, an order already known or a payload that cannot be read.
func prepareNewOrder(ctx *gofr.Context, record consumer.Record, seenOrders map[uuid.UUID]bool) *preparedOrder {
	message := record.Message
	book := orderbook.GetBook(message.Book)

	if message.Route != events.RouteNewOrder || book.IsApplied(message.ID) {
		return nil
	}

	var newOrder events.NewOrder
	if err := message.ParsePayload(&newOrder); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	if book.IsKnownOrder(newOrder.OrderID) || seenOrders[newOrder.OrderID] {
		return nil
	}

	seenOrders[newOrder.OrderID] = true
	currency, amount, err := getReservation(message.Book, newOrder)

	return &preparedOrder{newOrder: newOrder, currency: currency, amount: amount, err: err}
}

// The most the order can need: a limit buy its notional in the quote, a
// market buy its amount, and a sell its quantity in the base.
func getReservation(bookID string, newOrder events.NewOrder) (money.Currency, int64, error) {
	book, err := books.Normalize(bookID)
	if err != nil {
		return "", 0, err
	}

	if newOrder.Side == string(models.SideSell) {
		return book.Base, getValue(newOrder.Quantity), checkPresent(newOrder.Quantity)
	}

	if newOrder.Type == string(models.TypeMarket) {
		return book.Quote, getValue(newOrder.Amount), checkPresent(newOrder.Amount)
	}

	if err := checkPresent(newOrder.Quantity, newOrder.Limit); err != nil {
		return "", 0, err
	}

	notional, err := money.Notional(book.Base, *newOrder.Quantity, *newOrder.Limit)

	return book.Quote, notional, err
}

func checkPresent(values ...*int64) error {
	for _, value := range values {
		if value == nil {
			return errMissingAmount
		}
	}

	return nil
}

func getValue(value *int64) int64 {
	if value == nil {
		return 0
	}

	return *value
}

func applyNewOrder(ctx *gofr.Context, current *batch, applied *command) error {
	prepared, ok := current.orders[applied.record.Message.ID]
	if !ok {
		return nil
	}

	applied.book.MarkKnownOrder(prepared.newOrder.OrderID)
	applied.sequence = applied.book.NextSequence()
	newOrder := prepared.newOrder

	if prepared.err != nil || current.reservations[applied.record.Message.ID] != walletclient.ResultOK {
		current.emit(ctx, applied, events.RouteOrderRejected, newOrder.OrderID, newOrder.UserID, func(header events.EventHeader) any {
			return events.OrderRejected{EventHeader: header, Reason: getRejectionReason(prepared)}
		})

		return nil
	}

	current.emit(ctx, applied, events.RouteOrderAccepted, newOrder.OrderID, newOrder.UserID, func(header events.EventHeader) any {
		return events.OrderAccepted{EventHeader: header}
	})

	order := buildOrder(prepared, applied.sequence)
	if order.Type == models.TypeLimit {
		applied.book.Put(&order)

		return nil
	}

	cancelWithoutLiquidity(ctx, current, applied, prepared, order)

	return nil
}

func getRejectionReason(prepared *preparedOrder) string {
	if prepared.err != nil {
		return events.ReasonInvalidAmount
	}

	return events.ReasonInsufficientFunds
}

// A market order never rests: with no counterparty yet, it is cancelled and
// its whole reservation goes back to the wallet.
func cancelWithoutLiquidity(ctx *gofr.Context, current *batch, applied *command, prepared *preparedOrder, order models.Order) {
	current.release(applied.record.Message.ID, order, prepared.currency, order.Reserved)

	current.emit(ctx, applied, events.RouteOrderCancelled, order.ID, order.UserID, func(header events.EventHeader) any {
		return events.OrderCancelled{EventHeader: header, CancelledQuantity: order.Quantity, Released: order.Reserved, Reason: events.ReasonNoLiquidity}
	})
}

func buildOrder(prepared *preparedOrder, sequence uint64) models.Order {
	newOrder := prepared.newOrder

	return models.Order{
		ID: newOrder.OrderID, UserID: newOrder.UserID, Side: models.Side(newOrder.Side), Type: models.OrderType(newOrder.Type),
		Price: getValue(newOrder.Limit), Quantity: getValue(newOrder.Quantity), Reserved: prepared.amount, Sequence: sequence,
	}
}
