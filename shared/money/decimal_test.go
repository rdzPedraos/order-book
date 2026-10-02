package money

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func assertParsed(t *testing.T, currency Currency, input string, want int64) {
	t.Helper()

	c := require.New(t)

	got, err := Parse(currency, &input)
	c.NoError(err, "Parse(%s, %q)", currency, input)
	c.Equal(want, *got, "Parse(%s, %q)", currency, input)
}

func assertParseError(t *testing.T, currency Currency, input string, wantErr error) {
	t.Helper()

	c := require.New(t)

	_, err := Parse(currency, &input)
	c.ErrorIs(err, wantErr, "Parse(%s, %q)", currency, input)
}

func assertFormatted(t *testing.T, currency Currency, amount int64, want string) {
	t.Helper()

	c := require.New(t)

	got, err := Format(currency, amount)
	c.NoError(err, "Format(%s, %d)", currency, amount)
	c.Equal(want, got, "Format(%s, %d)", currency, amount)
}

func assertRoundTrip(t *testing.T, currency Currency, value string) {
	t.Helper()

	c := require.New(t)

	amount, err := Parse(currency, &value)
	c.NoError(err, "Parse(%s, %q)", currency, value)

	formatted, err := Format(currency, *amount)
	c.NoError(err, "Format(%s, %d)", currency, *amount)
	c.Equal(value, formatted, "round trip of %q", value)
}

func TestParse(t *testing.T) {
	t.Run("BRL with two decimals", func(t *testing.T) { assertParsed(t, BRL, "90.00", 9000) })
	t.Run("BRL with one decimal", func(t *testing.T) { assertParsed(t, BRL, "90.5", 9050) })
	t.Run("BRL without decimals", func(t *testing.T) { assertParsed(t, BRL, "90", 9000) })
	t.Run("BRL zero", func(t *testing.T) { assertParsed(t, BRL, "0", 0) })
	t.Run("BRL with too many decimals", func(t *testing.T) { assertParseError(t, BRL, "90.001", ErrTooManyDecimals) })
	t.Run("BRL negative", func(t *testing.T) { assertParseError(t, BRL, "-1.00", ErrNegative) })
	t.Run("BRL out of range", func(t *testing.T) { assertParseError(t, BRL, "92233720368547758.08", ErrOutOfRange) })
	t.Run("VIB integer", func(t *testing.T) { assertParsed(t, VIB, "10", 10) })
	t.Run("VIB fractional", func(t *testing.T) { assertParseError(t, VIB, "2.5", ErrTooManyDecimals) })
	t.Run("VIB negative", func(t *testing.T) { assertParseError(t, VIB, "-3", ErrNegative) })
	t.Run("empty input", func(t *testing.T) { assertParseError(t, BRL, "", ErrInvalidFormat) })
	t.Run("not a number", func(t *testing.T) { assertParseError(t, BRL, "abc", ErrInvalidFormat) })
	t.Run("dot without decimals", func(t *testing.T) { assertParseError(t, BRL, "90.", ErrInvalidFormat) })
	t.Run("unknown currency", func(t *testing.T) { assertParseError(t, Currency("XYZ"), "1", ErrUnknownCurrency) })
	t.Run("value not sent", func(t *testing.T) {
		c := require.New(t)

		amount, err := Parse(BRL, nil)
		c.NoError(err)
		c.Nil(amount)
	})
}

func TestFormat(t *testing.T) {
	t.Run("BRL cents", func(t *testing.T) { assertFormatted(t, BRL, 9000, "90.00") })
	t.Run("BRL less than one real", func(t *testing.T) { assertFormatted(t, BRL, 5, "0.05") })
	t.Run("BRL zero", func(t *testing.T) { assertFormatted(t, BRL, 0, "0.00") })
	t.Run("VIB units", func(t *testing.T) { assertFormatted(t, VIB, 10, "10") })

	t.Run("unknown currency", func(t *testing.T) {
		c := require.New(t)

		_, err := Format(Currency("XYZ"), 1)
		c.ErrorIs(err, ErrUnknownCurrency)
	})
}

func TestParseFormatRoundTrip(t *testing.T) {
	t.Run("BRL zero", func(t *testing.T) { assertRoundTrip(t, BRL, "0.00") })
	t.Run("BRL one cent", func(t *testing.T) { assertRoundTrip(t, BRL, "0.01") })
	t.Run("BRL with cents", func(t *testing.T) { assertRoundTrip(t, BRL, "90.00") })
	t.Run("BRL large amount", func(t *testing.T) { assertRoundTrip(t, BRL, "123456.78") })
	t.Run("VIB zero", func(t *testing.T) { assertRoundTrip(t, VIB, "0") })
	t.Run("VIB one", func(t *testing.T) { assertRoundTrip(t, VIB, "1") })
	t.Run("VIB large quantity", func(t *testing.T) { assertRoundTrip(t, VIB, "250000") })
}
