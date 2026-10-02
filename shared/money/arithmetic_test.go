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

func TestQuantityFor(t *testing.T) {
	t.Run("BRL-VIB: R$ 500.00 at R$ 110.00 buys 4 VIB", func(t *testing.T) {
		c := require.New(t)

		quantity, err := QuantityFor(VIB, 50000, 11000)
		c.NoError(err)
		c.Equal(int64(4), quantity)
	})

	t.Run("base with decimals buys a fraction", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 2)

		quantity, err := QuantityFor(base, 4500, 9000)
		c.NoError(err)
		c.Equal(int64(50), quantity)
	})

	t.Run("not enough for one unit", func(t *testing.T) {
		c := require.New(t)

		quantity, err := QuantityFor(VIB, 10999, 11000)
		c.NoError(err)
		c.Equal(int64(0), quantity)
	})

	t.Run("result that does not fit in int64", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 2)

		_, err := QuantityFor(base, math.MaxInt64, 1)
		c.ErrorIs(err, ErrOverflow)
	})

	t.Run("price that is not positive", func(t *testing.T) {
		c := require.New(t)

		_, err := QuantityFor(VIB, 50000, 0)
		c.ErrorIs(err, ErrOutOfRange)
	})

	t.Run("negative amount", func(t *testing.T) {
		c := require.New(t)

		_, err := QuantityFor(VIB, -1, 11000)
		c.ErrorIs(err, ErrNegative)
	})

	t.Run("unknown currency", func(t *testing.T) {
		c := require.New(t)

		_, err := QuantityFor("XXX", 1, 1)
		c.ErrorIs(err, ErrUnknownCurrency)
	})
}

func TestAveragePrice(t *testing.T) {
	t.Run("BRL-VIB: R$ 410.00 for 4 VIB averages R$ 102.50", func(t *testing.T) {
		c := require.New(t)

		average, err := AveragePrice(VIB, 41000, 4)
		c.NoError(err)
		c.Equal(int64(10250), average)
	})

	t.Run("half a cent rounds up", func(t *testing.T) {
		c := require.New(t)

		average, err := AveragePrice(VIB, 10001, 2)
		c.NoError(err)
		c.Equal(int64(5001), average)
	})

	t.Run("less than half a cent rounds down", func(t *testing.T) {
		c := require.New(t)

		average, err := AveragePrice(VIB, 10001, 3)
		c.NoError(err)
		c.Equal(int64(3334), average)
	})

	t.Run("base with decimals", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 2)

		average, err := AveragePrice(base, 4500, 50)
		c.NoError(err)
		c.Equal(int64(9000), average)
	})

	t.Run("nothing executed yet", func(t *testing.T) {
		c := require.New(t)

		average, err := AveragePrice(VIB, 0, 0)
		c.NoError(err)
		c.Equal(int64(0), average)
	})

	t.Run("result that does not fit in int64", func(t *testing.T) {
		c := require.New(t)
		base := withTestCurrency(t, 2)

		_, err := AveragePrice(base, math.MaxInt64, 1)
		c.ErrorIs(err, ErrOverflow)
	})

	t.Run("negative total or quantity", func(t *testing.T) {
		c := require.New(t)

		_, err := AveragePrice(VIB, -1, 1)
		c.ErrorIs(err, ErrNegative)

		_, err = AveragePrice(VIB, 1, -1)
		c.ErrorIs(err, ErrNegative)
	})

	t.Run("unknown currency", func(t *testing.T) {
		c := require.New(t)

		_, err := AveragePrice("XXX", 1, 1)
		c.ErrorIs(err, ErrUnknownCurrency)
	})
}
