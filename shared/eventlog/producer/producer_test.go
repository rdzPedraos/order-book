package producer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

func newMessage(c *require.Assertions, book string) events.Message {
	message, err := events.NewMessage("producer-test.Test", book, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), map[string]string{})
	c.NoError(err)

	return message
}

func TestMockPublish(t *testing.T) {
	t.Run("message is recorded", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		message := newMessage(c, "BRL-VIB")

		c.NoError(Publish(context.Background(), message))
		c.Equal([]events.Message{message}, mock.Messages)
	})

	t.Run("log unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("broker unreachable")

		c.ErrorIs(Publish(context.Background(), newMessage(c, "BRL-VIB")), mock.Err)
		c.Empty(mock.Messages)
	})

	t.Run("unknown book is not published", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)

		c.ErrorIs(Publish(context.Background(), newMessage(c, "BTC-USD")), books.ErrUnknownBook)
		c.Empty(mock.Messages)
	})
}

func TestPublishInvalidRoute(t *testing.T) {
	c := require.New(t)
	mock := InitMock(t)
	message := newMessage(c, "BRL-VIB")
	message.Route = "no-topic"

	c.ErrorIs(Publish(context.Background(), message), events.ErrInvalidRoute)
	c.Empty(mock.Messages)
}

func TestPublishWithoutConnection(t *testing.T) {
	c := require.New(t)

	c.ErrorIs(Publish(context.Background(), newMessage(c, "BRL-VIB")), ErrNotConnected)
}
