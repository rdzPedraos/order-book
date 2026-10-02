package books

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/shared/money"
)

func TestNormalize(t *testing.T) {
	want := Book{ID: "BRL-VIB", Base: money.VIB, Quote: money.BRL}

	t.Run("configured id", func(t *testing.T) {
		c := require.New(t)

		got, err := Normalize("BRL-VIB")
		c.NoError(err)
		c.Equal(want, got)
	})

	t.Run("lowercase id", func(t *testing.T) {
		c := require.New(t)

		got, err := Normalize("brl-vib")
		c.NoError(err)
		c.Equal(want, got)
	})

	t.Run("tickers in another order", func(t *testing.T) {
		c := require.New(t)

		_, err := Normalize("VIB-BRL")
		c.ErrorIs(err, ErrUnknownBook)
	})

	t.Run("unknown book", func(t *testing.T) {
		c := require.New(t)

		_, err := Normalize("BTC-USD")
		c.ErrorIs(err, ErrUnknownBook)
	})
}
