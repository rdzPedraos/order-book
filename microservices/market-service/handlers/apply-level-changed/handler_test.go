package applylevelchanged

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

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
	"github.com/rdzpedraos/order-book/microservices/market-service/store/marketdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

func newContext(t *testing.T) *gofr.Context {
	t.Helper()

	mockContainer, _ := container.NewMockContainer(t)
	mockContainer.Logger = logging.NewLogger(logging.FATAL)

	return &gofr.Context{Context: context.Background(), Container: mockContainer}
}

func levelMessage(c *require.Assertions, side string, price, volume int64, orders int) events.Message {
	changed := events.OrderBookLevelChanged{Side: side, Price: price, Volume: volume, Orders: orders}

	message, err := events.NewEventMessage(events.RouteOrderBookLevelChanged, "BRL-VIB", uuid.New(), 2, time.Now(), changed)
	c.NoError(err)

	return message
}

func TestHandle(t *testing.T) {
	t.Run("a new level", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)

		c.NoError(Handle(newContext(t), levelMessage(c, "BUY", 10000, 10, 1)))
		c.Equal([]models.Level{{Book: "BRL-VIB", Side: "BUY", Price: 10000, Volume: 10, Orders: 1}}, mock.Levels)
	})

	t.Run("a level that changes", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)

		c.NoError(Handle(newContext(t), levelMessage(c, "BUY", 10000, 10, 1)))
		c.NoError(Handle(newContext(t), levelMessage(c, "BUY", 10000, 30, 2)))
		c.Equal([]models.Level{{Book: "BRL-VIB", Side: "BUY", Price: 10000, Volume: 30, Orders: 2}}, mock.Levels)
	})

	t.Run("emptied level", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)

		c.NoError(Handle(newContext(t), levelMessage(c, "SELL", 8500, 4, 1)))
		c.NoError(Handle(newContext(t), levelMessage(c, "SELL", 8500, 0, 0)))
		c.Empty(mock.Levels)
	})

	t.Run("a repeated event writes the same level", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		message := levelMessage(c, "BUY", 10000, 10, 1)

		c.NoError(Handle(newContext(t), message))
		c.NoError(Handle(newContext(t), message))
		c.Len(mock.Levels, 1)
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(Handle(newContext(t), levelMessage(c, "BUY", 10000, 10, 1)), mock.Err)
	})

	t.Run("a payload that cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)

		c.NoError(Handle(newContext(t), events.Message{ID: uuid.New(), Route: events.RouteOrderBookLevelChanged, Payload: []byte(`[`)}))
		c.Empty(mock.Levels)
	})
}
