package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderValidate(t *testing.T) {
	t.Run("limit buy", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeLimit, Limit: ptr(9000), Quantity: ptr(10)}.Validate()
		c.NoError(err)
	})

	t.Run("limit sell", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideSell, Type: TypeLimit, Limit: ptr(9000), Quantity: ptr(10)}.Validate()
		c.NoError(err)
	})

	t.Run("market buy by amount", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeMarket, Amount: ptr(50000)}.Validate()
		c.NoError(err)
	})

	t.Run("market sell by quantity", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideSell, Type: TypeMarket, Quantity: ptr(10)}.Validate()
		c.NoError(err)
	})

	t.Run("invalid side", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: "HOLD", Type: TypeLimit, Limit: ptr(9000), Quantity: ptr(10)}.Validate()
		c.ErrorIs(err, ErrInvalidSide)
	})

	t.Run("limit without quantity", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideSell, Type: TypeLimit, Limit: ptr(9000)}.Validate()
		c.ErrorIs(err, ErrUnsupportedOrder)
	})

	t.Run("limit with amount", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeLimit, Limit: ptr(9000), Amount: ptr(50000)}.Validate()
		c.ErrorIs(err, ErrUnsupportedOrder)
	})

	t.Run("market buy without amount", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeMarket}.Validate()
		c.ErrorIs(err, ErrUnsupportedOrder)
	})

	t.Run("market buy by quantity", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeMarket, Quantity: ptr(10)}.Validate()
		c.ErrorIs(err, ErrUnsupportedOrder)
	})

	t.Run("market sell without quantity", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideSell, Type: TypeMarket}.Validate()
		c.ErrorIs(err, ErrUnsupportedOrder)
	})

	t.Run("market sell by amount", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideSell, Type: TypeMarket, Amount: ptr(50000)}.Validate()
		c.ErrorIs(err, ErrUnsupportedOrder)
	})

	t.Run("zero limit", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeLimit, Limit: ptr(0), Quantity: ptr(10)}.Validate()
		c.ErrorIs(err, ErrInvalidLimit)
	})

	t.Run("zero amount", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeMarket, Amount: ptr(0)}.Validate()
		c.ErrorIs(err, ErrInvalidAmount)
	})

	t.Run("zero quantity", func(t *testing.T) {
		c := require.New(t)

		err := Order{Side: SideBuy, Type: TypeLimit, Limit: ptr(9000), Quantity: ptr(0)}.Validate()
		c.ErrorIs(err, ErrInvalidQuantity)
	})
}
