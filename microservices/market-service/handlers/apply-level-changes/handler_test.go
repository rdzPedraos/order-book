package applylevelchanges

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

func deliver(t *testing.T, messages ...events.Message) error {
	t.Helper()

	return Handle(newContext(t), messages)
}

func TestHandle(t *testing.T) {
	t.Run("a new level", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)

		c.NoError(deliver(t, levelMessage(c, "BUY", 10000, 10, 1)))
		c.Equal([]models.Level{{Book: "BRL-VIB", Side: "BUY", Price: 10000, Volume: 10, Orders: 1}}, mock.Levels)
	})

	t.Run("a level that changes within a batch keeps its last state", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)

		c.NoError(deliver(t, levelMessage(c, "BUY", 10000, 10, 1), levelMessage(c, "BUY", 10000, 30, 2)))
		c.Equal([]models.Level{{Book: "BRL-VIB", Side: "BUY", Price: 10000, Volume: 30, Orders: 2}}, mock.Levels)
	})

	t.Run("emptied level", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)

		c.NoError(deliver(t, levelMessage(c, "SELL", 8500, 4, 1)))
		c.NoError(deliver(t, levelMessage(c, "SELL", 8500, 0, 0)))
		c.Empty(mock.Levels)
	})

	t.Run("a repeated event writes the same level", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		message := levelMessage(c, "BUY", 10000, 10, 1)

		c.NoError(deliver(t, message))
		c.NoError(deliver(t, message))
		c.Len(mock.Levels, 1)
	})

	t.Run("an unavailable database is retried", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(deliver(t, levelMessage(c, "BUY", 10000, 10, 1)), mock.Err)
	})

	t.Run("a payload that cannot be read is left out of its batch", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		unreadable := events.Message{ID: uuid.New(), Route: events.RouteOrderBookLevelChanged, Payload: []byte(`[`)}

		c.NoError(deliver(t, unreadable, levelMessage(c, "BUY", 10000, 10, 1)))
		c.Len(mock.Levels, 1)
	})

	t.Run("a batch without a readable level writes nothing", func(t *testing.T) {
		c := require.New(t)
		mock := marketdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.NoError(deliver(t, events.Message{ID: uuid.New(), Route: events.RouteOrderBookLevelChanged, Payload: []byte(`[`)}))
	})
}
