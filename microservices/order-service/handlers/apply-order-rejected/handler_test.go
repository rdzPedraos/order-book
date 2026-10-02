package applyorderrejected

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

func rejectedMessage(c *require.Assertions, orderID uuid.UUID) events.Message {
	rejected := events.OrderRejected{
		EventHeader:  events.EventHeader{CommandID: uuid.New(), OrderID: orderID, UserID: "ana"},
		OrderDetails: events.OrderDetails{Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10))},
		Reason:       events.ReasonInsufficientFunds,
	}

	message, err := events.NewEventMessage(events.RouteOrderRejected, "BRL-VIB", rejected.CommandID, 0, createdAt, rejected)
	c.NoError(err)

	return message
}

func pendingOrder(orderID uuid.UUID) models.Order {
	return models.Order{
		ID: orderID, UserID: "ana", Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit, Limit: ptr(int64(9000)),
		Quantity: ptr(int64(10)), Status: models.StatusPending, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func TestHandle(t *testing.T) {
	t.Run("rejection for funds", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		mock.Orders = []models.Order{pendingOrder(orderID)}

		c.NoError(Handle(newContext(t), rejectedMessage(c, orderID)))
		c.Equal(models.StatusRejected, mock.Orders[0].Status)
		c.Equal(ptr(events.ReasonInsufficientFunds), mock.Orders[0].Reason)
		c.Zero(mock.Orders[0].FilledQuantity)
	})

	t.Run("a rejection before the command stores the order rejected", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()

		c.NoError(Handle(newContext(t), rejectedMessage(c, orderID)))

		expected := pendingOrder(orderID)
		expected.Status, expected.Reason = models.StatusRejected, ptr(events.ReasonInsufficientFunds)
		c.Equal([]models.Order{expected}, mock.Orders)
	})

	t.Run("a repeated rejection changes nothing", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		orderID := uuid.New()
		message := rejectedMessage(c, orderID)

		c.NoError(Handle(newContext(t), message))
		c.NoError(Handle(newContext(t), message))
		c.Len(mock.Orders, 1)
		c.Equal(models.StatusRejected, mock.Orders[0].Status)
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(Handle(newContext(t), rejectedMessage(c, uuid.New())), mock.Err)
	})

	t.Run("a payload that cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)

		c.NoError(Handle(newContext(t), events.Message{ID: uuid.New(), Route: events.RouteOrderRejected, Payload: []byte(`[`)}))
		c.Empty(mock.Orders)
	})
}
