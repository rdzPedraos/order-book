package applyordermodified

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

func modifiedMessage(c *require.Assertions, orderID uuid.UUID, limit, quantity int64) events.Message {
	modified := events.OrderModified{EventHeader: events.EventHeader{CommandID: uuid.New(), OrderID: orderID, UserID: "ana"}, Limit: &limit, Quantity: &quantity}

	message, err := events.NewEventMessage(events.RouteOrderModified, "BRL-VIB", modified.CommandID, 0, createdAt, modified)
	c.NoError(err)

	return message
}

func storedOrder(orderID uuid.UUID, status models.Status, filled int64) models.Order {
	return models.Order{
		ID: orderID, UserID: "ana", Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit, Limit: ptr(int64(9000)),
		Quantity: ptr(int64(10)), FilledQuantity: filled, Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func TestHandle(t *testing.T) {
	t.Run("modification applied", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{storedOrder(orderID, models.StatusOpen, 0)}

		c.NoError(Handle(newContext(t), modifiedMessage(c, orderID, 9200, 10)))
		c.Equal(ptr(int64(9200)), mock.Orders[0].Limit)
		c.Equal(ptr(int64(10)), mock.Orders[0].Quantity)
	})

	t.Run("the engine's quantity is what is still pending", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{storedOrder(orderID, models.StatusPartiallyFilled, 4)}

		c.NoError(Handle(newContext(t), modifiedMessage(c, orderID, 9000, 2)))
		c.Equal(ptr(int64(6)), mock.Orders[0].Quantity, "4 executed plus 2 pending")
		c.Equal(models.StatusPartiallyFilled, mock.Orders[0].Status)
	})

	t.Run("a final order is not modified", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		cancelled := storedOrder(orderID, models.StatusCancelled, 0)
		mock.Orders = []models.Order{cancelled}

		c.NoError(Handle(newContext(t), modifiedMessage(c, orderID, 9200, 10)))
		c.Equal([]models.Order{cancelled}, mock.Orders)
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(Handle(newContext(t), modifiedMessage(c, uuid.New(), 9200, 10)), mock.Err)
	})

	t.Run("a payload that cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		orderdb.InitMock(t)

		c.NoError(Handle(newContext(t), events.Message{ID: uuid.New(), Route: events.RouteOrderModified, Payload: []byte(`[`)}))
	})

	t.Run("an event without its limit or quantity is skipped", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{storedOrder(orderID, models.StatusOpen, 0)}
		modified := events.OrderModified{EventHeader: events.EventHeader{OrderID: orderID}}
		message, err := events.NewEventMessage(events.RouteOrderModified, "BRL-VIB", uuid.New(), 0, createdAt, modified)
		c.NoError(err)

		c.NoError(Handle(newContext(t), message))
		c.Equal(ptr(int64(9000)), mock.Orders[0].Limit)
	})
}
