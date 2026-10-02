package applytradeexecuted

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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

func newContext(t *testing.T) *gofr.Context {
	t.Helper()

	mockContainer, _ := container.NewMockContainer(t)
	mockContainer.Logger = logging.NewLogger(logging.FATAL)

	return &gofr.Context{Context: context.Background(), Container: mockContainer}
}

func openOrder(side models.Side, orderType models.OrderType, quantity, amount *int64) models.Order {
	status := models.StatusOpen
	if orderType == models.TypeMarket {
		status = models.StatusPending
	}

	return models.Order{
		ID: uuid.New(), UserID: "ana", Book: "BRL-VIB", Side: side, Type: orderType,
		Quantity: quantity, Amount: amount, Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func tradeMessage(c *require.Assertions, buy, sell models.Order, quantity, price int64) events.Message {
	trade := events.TradeExecuted{
		TradeID: uuid.New(), BuyOrderID: buy.ID, SellOrderID: sell.ID,
		Price: price, Quantity: quantity, Amount: quantity * price,
	}

	message, err := events.NewEventMessage(events.RouteTradeExecuted, "BRL-VIB", uuid.New(), 1, createdAt, trade)
	c.NoError(err)

	return message
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
	t.Run("limit filled when it arrives", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy, sell := openOrder(models.SideBuy, models.TypeLimit, ptr(int64(10)), nil), openOrder(models.SideSell, models.TypeLimit, ptr(int64(10)), nil)
		mock.Orders = []models.Order{buy, sell}

		c.NoError(Handle(newContext(t), tradeMessage(c, buy, sell, 10, 9000)))

		c.Equal(models.StatusFilled, getOrder(mock, buy.ID).Status)
		c.Equal(models.StatusFilled, getOrder(mock, sell.ID).Status)
		c.Equal(int64(90000), getOrder(mock, buy.ID).FilledAmount)
	})

	t.Run("limit filled in parts", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy := openOrder(models.SideBuy, models.TypeLimit, ptr(int64(10)), nil)
		first, second := openOrder(models.SideSell, models.TypeLimit, ptr(int64(4)), nil), openOrder(models.SideSell, models.TypeLimit, ptr(int64(6)), nil)
		mock.Orders = []models.Order{buy, first, second}

		c.NoError(Handle(newContext(t), tradeMessage(c, buy, first, 4, 8500)))
		c.Equal(models.StatusPartiallyFilled, getOrder(mock, buy.ID).Status)
		c.Equal(int64(4), getOrder(mock, buy.ID).FilledQuantity)

		c.NoError(Handle(newContext(t), tradeMessage(c, buy, second, 6, 9000)))
		c.Equal(models.StatusFilled, getOrder(mock, buy.ID).Status)
		c.Equal(int64(10), getOrder(mock, buy.ID).FilledQuantity)
	})

	t.Run("a market buy stays pending until its amount is spent", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy := openOrder(models.SideBuy, models.TypeMarket, nil, ptr(int64(50000)))
		first, second := openOrder(models.SideSell, models.TypeLimit, ptr(int64(3)), nil), openOrder(models.SideSell, models.TypeLimit, ptr(int64(1)), nil)
		mock.Orders = []models.Order{buy, first, second}

		c.NoError(Handle(newContext(t), tradeMessage(c, buy, first, 3, 10000)))
		c.NoError(Handle(newContext(t), tradeMessage(c, buy, second, 1, 11000)))

		c.Equal(models.StatusPending, getOrder(mock, buy.ID).Status, "its remainder is cancelled by its own event")
		c.Equal(int64(41000), getOrder(mock, buy.ID).FilledAmount)
	})

	t.Run("a market buy that spends its whole amount is filled", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy, sell := openOrder(models.SideBuy, models.TypeMarket, nil, ptr(int64(30000))), openOrder(models.SideSell, models.TypeLimit, ptr(int64(5)), nil)
		mock.Orders = []models.Order{buy, sell}

		c.NoError(Handle(newContext(t), tradeMessage(c, buy, sell, 3, 10000)))

		c.Equal(models.StatusFilled, getOrder(mock, buy.ID).Status)
		c.Equal(models.StatusPartiallyFilled, getOrder(mock, sell.ID).Status)
	})

	t.Run("duplicated event", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		buy, sell := openOrder(models.SideBuy, models.TypeLimit, ptr(int64(10)), nil), openOrder(models.SideSell, models.TypeLimit, ptr(int64(10)), nil)
		mock.Orders = []models.Order{buy, sell}
		trade := tradeMessage(c, buy, sell, 4, 9000)

		c.NoError(Handle(newContext(t), trade))
		c.NoError(Handle(newContext(t), trade))

		c.Equal(int64(4), getOrder(mock, buy.ID).FilledQuantity)
		c.Len(mock.Trades, 1)
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")
		buy, sell := openOrder(models.SideBuy, models.TypeLimit, ptr(int64(10)), nil), openOrder(models.SideSell, models.TypeLimit, ptr(int64(10)), nil)

		c.ErrorIs(Handle(newContext(t), tradeMessage(c, buy, sell, 4, 9000)), mock.Err)
	})

	t.Run("a payload that cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)

		c.NoError(Handle(newContext(t), events.Message{ID: uuid.New(), Route: events.RouteTradeExecuted, Payload: []byte(`[`)}))
		c.Empty(mock.Trades)
	})
}
