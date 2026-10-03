// Package applyordermessages handles the projector's batches of the log: it
// keeps in orders each NewOrder of orders.commands and each event of
// orders.events that changes an order, applying the whole batch at once.
package applyordermessages

import (
	"errors"
	"slices"
	"time"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// LagMetric is registered in main.go.
const LagMetric = "order_projection_lag_seconds"

var (
	errMissingTerms = errors.New("an OrderModified needs its limit and quantity")
	errUnknownRoute = errors.New("not a route of the projector")
)

var Routes = []string{
	events.RouteNewOrder, events.RouteOrderAccepted, events.RouteOrderRejected,
	events.RouteOrderCancelled, events.RouteOrderModified, events.RouteTradeExecuted,
}

func Handle(ctx *gofr.Context, messages []events.Message) error {
	changes := buildChanges(ctx, messages)
	if len(changes) == 0 {
		return nil
	}

	if err := orderdb.ApplyChanges(ctx, changes); err != nil {
		return err
	}

	setProjectionLag(ctx, messages)

	return nil
}

// A message that cannot be read never will, so it is logged and left out.
func buildChanges(ctx *gofr.Context, messages []events.Message) []orderdb.Change {
	changes := make([]orderdb.Change, 0, len(messages))

	for _, message := range messages {
		change, err := buildChange(message)
		if err != nil {
			ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

			continue
		}

		changes = append(changes, *change)
	}

	return changes
}

func buildChange(message events.Message) (*orderdb.Change, error) {
	switch message.Route {
	case events.RouteNewOrder:
		return buildInsert(message)
	case events.RouteOrderAccepted:
		return buildAccepted(message)
	case events.RouteOrderRejected:
		return buildRejected(message)
	case events.RouteOrderCancelled:
		return buildCancel(message)
	case events.RouteOrderModified:
		return buildModify(message)
	case events.RouteTradeExecuted:
		return buildTrade(message)
	default:
		return nil, errUnknownRoute
	}
}

func buildInsert(message events.Message) (*orderdb.Change, error) {
	var newOrder events.NewOrder
	if err := message.ParsePayload(&newOrder); err != nil {
		return nil, err
	}

	order := models.Order{
		ID: newOrder.OrderID, UserID: newOrder.UserID, Book: message.Book, Side: models.Side(newOrder.Side),
		Type: models.OrderType(newOrder.Type), Limit: newOrder.Limit, Amount: newOrder.Amount, Quantity: newOrder.Quantity,
		Status: models.StatusPending, CreatedAt: message.CreatedAt, UpdatedAt: message.CreatedAt,
	}

	return &orderdb.Change{Kind: orderdb.ChangeInsert, Order: &order}, nil
}

// A market order stays PENDING until it is filled or its remainder is cancelled.
func buildAccepted(message events.Message) (*orderdb.Change, error) {
	var accepted events.OrderAccepted
	if err := message.ParsePayload(&accepted); err != nil {
		return nil, err
	}

	status := models.StatusOpen
	if accepted.Type == string(models.TypeMarket) {
		status = models.StatusPending
	}

	order := models.NewOrderFromDetails(accepted.EventHeader, message.Book, accepted.OrderDetails, status, message.CreatedAt)

	return &orderdb.Change{Kind: orderdb.ChangeFirstEvent, Order: &order}, nil
}

func buildRejected(message events.Message) (*orderdb.Change, error) {
	var rejected events.OrderRejected
	if err := message.ParsePayload(&rejected); err != nil {
		return nil, err
	}

	order := models.NewOrderFromDetails(rejected.EventHeader, message.Book, rejected.OrderDetails, models.StatusRejected, message.CreatedAt)
	order.Reason = &rejected.Reason

	return &orderdb.Change{Kind: orderdb.ChangeFirstEvent, Order: &order}, nil
}

// An empty reason is a cancellation the person asked for.
func buildCancel(message events.Message) (*orderdb.Change, error) {
	var cancelled events.OrderCancelled
	if err := message.ParsePayload(&cancelled); err != nil {
		return nil, err
	}

	var reason *string
	if cancelled.Reason != "" {
		reason = &cancelled.Reason
	}

	return &orderdb.Change{Kind: orderdb.ChangeCancel, OrderID: cancelled.OrderID, Reason: reason, At: message.CreatedAt}, nil
}

// The engine always sends both: the new limit and what is still pending.
func buildModify(message events.Message) (*orderdb.Change, error) {
	var modified events.OrderModified
	if err := message.ParsePayload(&modified); err != nil {
		return nil, err
	}

	if modified.Limit == nil || modified.Quantity == nil {
		return nil, errMissingTerms
	}

	return &orderdb.Change{
		Kind: orderdb.ChangeModify, OrderID: modified.OrderID, Limit: *modified.Limit,
		PendingQuantity: *modified.Quantity, At: message.CreatedAt,
	}, nil
}

func buildTrade(message events.Message) (*orderdb.Change, error) {
	var executed events.TradeExecuted
	if err := message.ParsePayload(&executed); err != nil {
		return nil, err
	}

	trade := models.Trade{
		ID: executed.TradeID, BuyOrderID: executed.BuyOrderID, SellOrderID: executed.SellOrderID,
		Price: executed.Price, Quantity: executed.Quantity, Amount: executed.Amount, CreatedAt: message.CreatedAt,
	}

	return &orderdb.Change{Kind: orderdb.ChangeTrade, Trade: &trade}, nil
}

// Measured from the newest NewOrder of the batch, the time its command was
// accepted, so it shows how far behind the projector is.
func setProjectionLag(ctx *gofr.Context, messages []events.Message) {
	for _, message := range slices.Backward(messages) {
		if message.Route == events.RouteNewOrder {
			ctx.Metrics().SetGauge(LagMetric, time.Since(message.CreatedAt).Seconds())

			return
		}
	}
}
