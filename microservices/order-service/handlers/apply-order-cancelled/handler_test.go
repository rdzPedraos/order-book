package applyordercancelled

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

func cancelledMessage(c *require.Assertions, orderID uuid.UUID, reason string) events.Message {
	cancelled := events.OrderCancelled{EventHeader: events.EventHeader{CommandID: uuid.New(), OrderID: orderID, UserID: "ana"}, Reason: reason}

	message, err := events.NewEventMessage(events.RouteOrderCancelled, "BRL-VIB", cancelled.CommandID, 0, createdAt, cancelled)
	c.NoError(err)

	return message
}

func storedOrder(orderID uuid.UUID, status models.Status) models.Order {
	return models.Order{
		ID: orderID, UserID: "ana", Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit, Limit: ptr(int64(9000)),
		Quantity: ptr(int64(10)), Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func TestHandle(t *testing.T) {
	t.Run("an open order is cancelled", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{storedOrder(orderID, models.StatusOpen)}

		c.NoError(Handle(newContext(t), cancelledMessage(c, orderID, "")))
		c.Equal(models.StatusCancelled, mock.Orders[0].Status)
		c.Nil(mock.Orders[0].Reason)
	})

	t.Run("a market order cancelled for lack of liquidity keeps its reason", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{storedOrder(orderID, models.StatusPending)}

		c.NoError(Handle(newContext(t), cancelledMessage(c, orderID, events.ReasonNoLiquidity)))
		c.Equal(models.StatusCancelled, mock.Orders[0].Status)
		c.Equal(ptr(events.ReasonNoLiquidity), mock.Orders[0].Reason)
	})

	t.Run("cancellation that arrives late", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		filled := storedOrder(orderID, models.StatusFilled)
		mock.Orders = []models.Order{filled}

		c.NoError(Handle(newContext(t), cancelledMessage(c, orderID, "")))
		c.Equal([]models.Order{filled}, mock.Orders)
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(Handle(newContext(t), cancelledMessage(c, uuid.New(), "")), mock.Err)
	})

	t.Run("a payload that cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		orderdb.InitMock(t)

		c.NoError(Handle(newContext(t), events.Message{ID: uuid.New(), Route: events.RouteOrderCancelled, Payload: []byte(`[`)}))
	})
}
