package insertneworder

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

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

var (
	orderID    = uuid.MustParse("01923456-7890-7abc-8def-0123456789ab")
	acceptedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
)

func ptr[T any](value T) *T {
	return &value
}

func newOrderMessage(c *require.Assertions, createdAt time.Time, newOrder events.NewOrder) events.Message {
	newOrder.OrderID, newOrder.UserID = orderID, "user-a"

	message, err := events.NewMessage(events.RouteNewOrder, "BRL-VIB", createdAt, newOrder)
	c.NoError(err)

	return message
}

func contextFor(t *testing.T, ctx context.Context) (*gofr.Context, *container.Mocks) {
	c, mocks := container.NewMockContainer(t)

	return &gofr.Context{Context: ctx, Container: c}, mocks
}

func deliver(t *testing.T, message events.Message) error {
	gofrCtx, mocks := contextFor(t, context.Background())
	mocks.Metrics.EXPECT().SetGauge(LagMetric, gomock.Any()).AnyTimes()

	return Handle(gofrCtx, message)
}

func TestHandle(t *testing.T) {
	t.Run("order visible once recorded", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)

		message := newOrderMessage(c, acceptedAt, events.NewOrder{
			Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10)),
		})

		c.NoError(deliver(t, message))
		c.Equal([]models.Order{{
			ID: orderID, UserID: "user-a", Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit,
			Limit: ptr(int64(9000)), Quantity: ptr(int64(10)), Status: models.StatusPending,
			CreatedAt: acceptedAt, UpdatedAt: acceptedAt,
		}}, mock.Orders)
	})

	t.Run("command delivered twice", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)

		message := newOrderMessage(c, acceptedAt, events.NewOrder{Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))})

		c.NoError(deliver(t, message))
		c.NoError(deliver(t, message))
		c.Len(mock.Orders, 1)
	})

	t.Run("malformed new order is skipped", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)

		malformed := events.Message{Route: events.RouteNewOrder, Book: "BRL-VIB", Payload: json.RawMessage(`{"limit":9000}`)}

		c.NoError(deliver(t, malformed))
		c.Empty(mock.Orders)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		err := deliver(t, newOrderMessage(c, acceptedAt, events.NewOrder{Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))}))
		c.ErrorIs(err, mock.Err)
		c.Empty(mock.Orders)
	})

	t.Run("projection lag is measured from the acceptance", func(t *testing.T) {
		c := require.New(t)
		orderdb.InitMock(t)

		gofrCtx, mocks := contextFor(t, context.Background())
		recent := newOrderMessage(c, time.Now().Add(-2*time.Second), events.NewOrder{Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))})

		mocks.Metrics.EXPECT().SetGauge(LagMetric, gomock.Any()).Do(func(_ string, lag float64, _ ...string) {
			c.InDelta(2, lag, 1)
		})

		c.NoError(Handle(gofrCtx, recent))
	})
}
