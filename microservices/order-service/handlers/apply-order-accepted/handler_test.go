package applyorderaccepted

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

func acceptedMessage(c *require.Assertions, orderID uuid.UUID, details events.OrderDetails) events.Message {
	accepted := events.OrderAccepted{EventHeader: events.EventHeader{CommandID: uuid.New(), OrderID: orderID, UserID: "ana"}, OrderDetails: details}

	message, err := events.NewEventMessage(events.RouteOrderAccepted, "BRL-VIB", accepted.CommandID, 0, createdAt, accepted)
	c.NoError(err)

	return message
}

func limitBuy() events.OrderDetails {
	return events.OrderDetails{Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10))}
}

func pendingOrder(orderID uuid.UUID, orderType models.OrderType) models.Order {
	return models.Order{
		ID: orderID, UserID: "ana", Book: "BRL-VIB", Side: models.SideBuy, Type: orderType, Limit: ptr(int64(9000)),
		Quantity: ptr(int64(10)), Status: models.StatusPending, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func TestHandle(t *testing.T) {
	t.Run("a limit order is open once accepted", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{pendingOrder(orderID, models.TypeLimit)}

		c.NoError(Handle(newContext(t), acceptedMessage(c, orderID, limitBuy())))
		c.Equal(models.StatusOpen, mock.Orders[0].Status)
	})

	t.Run("a market order stays pending once accepted", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{pendingOrder(orderID, models.TypeMarket)}

		c.NoError(Handle(newContext(t), acceptedMessage(c, orderID, events.OrderDetails{Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))})))
		c.Equal(models.StatusPending, mock.Orders[0].Status)
	})

	t.Run("event before the command", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()

		c.NoError(Handle(newContext(t), acceptedMessage(c, orderID, limitBuy())))

		expected := pendingOrder(orderID, models.TypeLimit)
		expected.Status = models.StatusOpen
		c.Equal([]models.Order{expected}, mock.Orders)

		c.NoError(orderdb.InsertOrder(newContext(t), pendingOrder(orderID, models.TypeLimit)))
		c.Equal([]models.Order{expected}, mock.Orders, "the NewOrder that arrives later does not change it")
	})

	t.Run("a repeated acceptance after a trade changes nothing", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		partial := pendingOrder(orderID, models.TypeLimit)
		partial.Status, partial.FilledQuantity = models.StatusPartiallyFilled, 4
		mock.Orders = []models.Order{partial}

		c.NoError(Handle(newContext(t), acceptedMessage(c, orderID, limitBuy())))
		c.Equal([]models.Order{partial}, mock.Orders)
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(Handle(newContext(t), acceptedMessage(c, uuid.New(), limitBuy())), mock.Err)
	})

	t.Run("a payload that cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)

		c.NoError(Handle(newContext(t), events.Message{ID: uuid.New(), Route: events.RouteOrderAccepted, Payload: []byte(`[`)}))
		c.Empty(mock.Orders)
	})
}
