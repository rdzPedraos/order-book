package orderbook

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestState(t *testing.T) {
	t.Run("each book has its own state, kept between calls", func(t *testing.T) {
		c := require.New(t)
		InitEmpty(t)

		GetBook("BRL-VIB").NextSequence()

		c.Equal(uint64(1), GetBook("BRL-VIB").GetSequence())
		c.Equal(uint64(0), GetBook("USD-VIB").GetSequence())
	})

	t.Run("sequence grows by one", func(t *testing.T) {
		c := require.New(t)
		book := New()

		c.Equal(uint64(1), book.NextSequence())
		c.Equal(uint64(2), book.NextSequence())
	})

	t.Run("applied messages and known orders are remembered", func(t *testing.T) {
		c := require.New(t)
		book := New()
		messageID, orderID := uuid.New(), uuid.New()

		c.False(book.IsApplied(messageID))
		book.MarkApplied(messageID)
		c.True(book.IsApplied(messageID))

		c.False(book.IsKnownOrder(orderID))
		book.MarkKnownOrder(orderID)
		c.True(book.IsKnownOrder(orderID))
	})
}
