package money

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// A base currency with decimals, which VIB does not have today.
func withTestCurrency(t *testing.T, decimals int) Currency {
	t.Helper()

	const test Currency = "XTS"
	decimalByCurrency[test] = decimals
	t.Cleanup(func() { delete(decimalByCurrency, test) })

	return test
}

func TestNotional(t *testing.T) {
	t.Run("BRL-VIB: 10 VIB at R$ 90.00", func(t *testing.T) {
		c := require.New(t)

		notional, err := Notional(VIB, 10, 9000)
		c.NoError(err)
		c.Equal(int64(90000), notional)
	})

	t.Run("base with decimals, exact division", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 2)

		notional, err := Notional(base, 150, 1000)
		c.NoError(err)
		c.Equal(int64(1500), notional)
	})

	t.Run("base with decimals, rounded up", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 2)

		notional, err := Notional(base, 1, 1)
		c.NoError(err)
		c.Equal(int64(1), notional)
	})

	t.Run("product larger than int64 with a result that fits", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 2)

		notional, err := Notional(base, 100_000_000_000_000_000, 1000)
		c.NoError(err)
		c.Equal(int64(1_000_000_000_000_000_000), notional)
	})

	t.Run("result that does not fit in int64", func(t *testing.T) {
		c := require.New(t)

		_, err := Notional(VIB, math.MaxInt64, 2)
		c.ErrorIs(err, ErrOverflow)
	})

	t.Run("product too large to divide", func(t *testing.T) {
		c := require.New(t)

		_, err := Notional(VIB, math.MaxInt64, math.MaxInt64)
		c.ErrorIs(err, ErrOverflow)
	})

	t.Run("rounding up past int64", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 1)

		_, err := Notional(base, math.MaxInt64, 10)
		c.NoError(err)

		_, err = Notional(base, math.MaxInt64, 11)
		c.ErrorIs(err, ErrOverflow)
	})

	t.Run("negative quantity or price", func(t *testing.T) {
		c := require.New(t)

		_, err := Notional(VIB, -1, 9000)
		c.ErrorIs(err, ErrNegative)

		_, err = Notional(VIB, 1, -9000)
		c.ErrorIs(err, ErrNegative)
	})

	t.Run("unknown currency", func(t *testing.T) {
		c := require.New(t)

		_, err := Notional("XXX", 1, 1)
		c.ErrorIs(err, ErrUnknownCurrency)
	})
}
