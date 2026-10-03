package applyordermessages

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/logging"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

var createdAt = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func ptr[T any](value T) *T {
	return &value
}

func newContext(t *testing.T) (*gofr.Context, *container.Mocks) {
	t.Helper()

	mockContainer, mocks := container.NewMockContainer(t)
	mockContainer.Logger = logging.NewLogger(logging.FATAL)

	return &gofr.Context{Context: context.Background(), Container: mockContainer}, mocks
}

func deliver(t *testing.T, messages ...events.Message) error {
	t.Helper()

	ctx, mocks := newContext(t)
	mocks.Metrics.EXPECT().SetGauge(LagMetric, gomock.Any()).AnyTimes()

	return Handle(ctx, messages)
}

func newOrderMessage(c *require.Assertions, orderID uuid.UUID, sentAt time.Time, details events.OrderDetails) events.Message {
	newOrder := events.NewOrder{
		OrderID: orderID, UserID: "ana", Side: details.Side, Type: details.Type,
		Limit: details.Limit, Amount: details.Amount, Quantity: details.Quantity,
	}

	message, err := events.NewMessage(events.RouteNewOrder, "BRL-VIB", sentAt, newOrder)
	c.NoError(err)

	return message
}

func newEventMessage(c *require.Assertions, route string, payload any) events.Message {
	message, err := events.NewEventMessage(route, "BRL-VIB", uuid.New(), 0, createdAt, payload)
	c.NoError(err)

	return message
}

func header(orderID uuid.UUID) events.EventHeader {
	return events.EventHeader{CommandID: uuid.New(), OrderID: orderID, UserID: "ana"}
}

func acceptedMessage(c *require.Assertions, orderID uuid.UUID, details events.OrderDetails) events.Message {
	return newEventMessage(c, events.RouteOrderAccepted, events.OrderAccepted{EventHeader: header(orderID), OrderDetails: details})
}

func cancelledMessage(c *require.Assertions, orderID uuid.UUID, reason string) events.Message {
	return newEventMessage(c, events.RouteOrderCancelled, events.OrderCancelled{EventHeader: header(orderID), Reason: reason})
}

func tradeMessage(c *require.Assertions, buy, sell models.Order, quantity, price int64) events.Message {
	return newEventMessage(c, events.RouteTradeExecuted, events.TradeExecuted{
		TradeID: uuid.New(), BuyOrderID: buy.ID, SellOrderID: sell.ID, Price: price, Quantity: quantity, Amount: quantity * price,
	})
}

func limitBuy(quantity int64) events.OrderDetails {
	return events.OrderDetails{Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(quantity)}
}

func storedOrder(side models.Side, orderType models.OrderType, status models.Status) models.Order {
	return models.Order{
		ID: uuid.New(), UserID: "ana", Book: "BRL-VIB", Side: side, Type: orderType, Limit: ptr(int64(9000)),
		Quantity: ptr(int64(10)), Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func getOrder(mock *orderdb.Mock, id uuid.UUID) models.Order {
	for _, order := range mock.Orders {
		if order.ID == id {
			return order
		}
	}

	return models.Order{}
}

func TestHandle(t *testing.T) {
	t.Run("order visible once recorded", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()

		c.NoError(deliver(t, newOrderMessage(c, orderID, createdAt, limitBuy(10))))

		c.Equal([]models.Order{{
			ID: orderID, UserID: "ana", Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit,
			Limit: ptr(int64(9000)), Quantity: ptr(int64(10)), Status: models.StatusPending,
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}}, mock.Orders)
	})

	t.Run("a limit order is open once accepted", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()

		c.NoError(deliver(t, newOrderMessage(c, orderID, createdAt, limitBuy(10)), acceptedMessage(c, orderID, limitBuy(10))))

		c.Equal(models.StatusOpen, getOrder(mock, orderID).Status)
	})

	t.Run("a market order stays pending once accepted", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()

		c.NoError(deliver(t, acceptedMessage(c, orderID, events.OrderDetails{Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))})))

		c.Equal(models.StatusPending, getOrder(mock, orderID).Status)
	})

	t.Run("event before the command", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()

		c.NoError(deliver(t, acceptedMessage(c, orderID, limitBuy(10)), newOrderMessage(c, orderID, createdAt, limitBuy(10))))

		c.Equal([]any{models.StatusOpen, ptr(int64(10))}, []any{getOrder(mock, orderID).Status, getOrder(mock, orderID).Quantity})
		c.Len(mock.Orders, 1)
	})

	t.Run("rejection for funds", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()

		c.NoError(deliver(t, newEventMessage(c, events.RouteOrderRejected,
			events.OrderRejected{EventHeader: header(orderID), OrderDetails: limitBuy(10), Reason: "insufficient_funds"})))

		c.Equal([]any{models.StatusRejected, ptr("insufficient_funds")}, []any{getOrder(mock, orderID).Status, getOrder(mock, orderID).Reason})
		c.Empty(mock.Trades)
	})

	t.Run("modification applied", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		open := storedOrder(models.SideBuy, models.TypeLimit, models.StatusOpen)
		mock.Orders = []models.Order{open}

		c.NoError(deliver(t, newEventMessage(c, events.RouteOrderModified,
			events.OrderModified{EventHeader: header(open.ID), Limit: ptr(int64(9200)), Quantity: ptr(int64(10))})))

		c.Equal(int64(9200), *getOrder(mock, open.ID).Limit)
	})

	t.Run("cancellation that arrives late", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		filled := storedOrder(models.SideBuy, models.TypeLimit, models.StatusFilled)
		mock.Orders = []models.Order{filled}

		c.NoError(deliver(t, cancelledMessage(c, filled.ID, "")))

		c.Equal(filled, getOrder(mock, filled.ID))
	})

	t.Run("limit filled when it arrives", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy, sell := storedOrder(models.SideBuy, models.TypeLimit, models.StatusOpen), storedOrder(models.SideSell, models.TypeLimit, models.StatusOpen)
		mock.Orders = []models.Order{buy, sell}

		c.NoError(deliver(t, tradeMessage(c, buy, sell, 10, 9000)))

		c.Equal(models.StatusFilled, getOrder(mock, buy.ID).Status)
		c.Equal(models.StatusFilled, getOrder(mock, sell.ID).Status)
	})

	t.Run("limit filled in parts", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy, sell := storedOrder(models.SideBuy, models.TypeLimit, models.StatusOpen), storedOrder(models.SideSell, models.TypeLimit, models.StatusOpen)
		mock.Orders = []models.Order{buy, sell}

		c.NoError(deliver(t, tradeMessage(c, buy, sell, 4, 9000)))
		c.Equal([]any{models.StatusPartiallyFilled, int64(4)}, []any{getOrder(mock, buy.ID).Status, getOrder(mock, buy.ID).FilledQuantity})

		c.NoError(deliver(t, tradeMessage(c, buy, sell, 6, 9000)))
		c.Equal([]any{models.StatusFilled, int64(10)}, []any{getOrder(mock, buy.ID).Status, getOrder(mock, buy.ID).FilledQuantity})
	})

	t.Run("market with a remainder", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy, sell := storedOrder(models.SideBuy, models.TypeMarket, models.StatusPending), storedOrder(models.SideSell, models.TypeLimit, models.StatusOpen)
		buy.Limit, buy.Quantity, buy.Amount = nil, nil, ptr(int64(50000))
		mock.Orders = []models.Order{buy, sell}

		c.NoError(deliver(t, tradeMessage(c, buy, sell, 4, 10250), cancelledMessage(c, buy.ID, "no_liquidity")))

		c.Equal([]any{models.StatusCancelled, int64(41000), ptr("no_liquidity")},
			[]any{getOrder(mock, buy.ID).Status, getOrder(mock, buy.ID).FilledAmount, getOrder(mock, buy.ID).Reason})
	})

	t.Run("duplicated event", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy, sell := storedOrder(models.SideBuy, models.TypeLimit, models.StatusOpen), storedOrder(models.SideSell, models.TypeLimit, models.StatusOpen)
		mock.Orders = []models.Order{buy, sell}
		trade := tradeMessage(c, buy, sell, 4, 9000)

		c.NoError(deliver(t, trade, trade))

		c.Equal(int64(4), getOrder(mock, buy.ID).FilledQuantity)
	})

	t.Run("a batch of several routes is applied in one call, in order", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buyID, sellID := uuid.New(), uuid.New()
		sellDetails := events.OrderDetails{Side: "SELL", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(4))}

		c.NoError(deliver(t,
			newOrderMessage(c, buyID, createdAt, limitBuy(10)),
			newOrderMessage(c, sellID, createdAt, sellDetails),
			acceptedMessage(c, buyID, limitBuy(10)),
			acceptedMessage(c, sellID, sellDetails),
			tradeMessage(c, models.Order{ID: buyID}, models.Order{ID: sellID}, 4, 9000),
			cancelledMessage(c, buyID, ""),
		))

		c.Equal(models.StatusCancelled, getOrder(mock, buyID).Status)
		c.Equal(models.StatusFilled, getOrder(mock, sellID).Status)
	})

	t.Run("a message that cannot be read is left out of its batch", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		malformed := events.Message{Route: events.RouteNewOrder, Book: "BRL-VIB", Payload: json.RawMessage(`{"limit":9000}`)}
		withoutTerms := newEventMessage(c, events.RouteOrderModified, events.OrderModified{EventHeader: header(orderID)})

		c.NoError(deliver(t, malformed, newOrderMessage(c, orderID, createdAt, limitBuy(10)), withoutTerms))

		c.Equal([]any{1, ptr(int64(10))}, []any{len(mock.Orders), getOrder(mock, orderID).Quantity})
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(deliver(t, newOrderMessage(c, uuid.New(), createdAt, limitBuy(10))), mock.Err)
	})

	t.Run("projection lag is measured from the last new order", func(t *testing.T) {
		c := require.New(t)
		orderdb.InitMock(t)
		ctx, mocks := newContext(t)

		mocks.Metrics.EXPECT().SetGauge(LagMetric, gomock.Any()).Do(func(_ string, lag float64, _ ...string) {
			c.InDelta(2, lag, 1)
		})

		c.NoError(Handle(ctx, []events.Message{
			newOrderMessage(c, uuid.New(), time.Now().Add(-time.Hour), limitBuy(10)),
			newOrderMessage(c, uuid.New(), time.Now().Add(-2*time.Second), limitBuy(10)),
		}))
	})
}
