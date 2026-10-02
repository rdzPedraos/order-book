// Package money represents amounts as int64 in each currency's minimal unit
// and converts them from and to the decimal strings used by the APIs.
//
// Floats are avoided because binary floating point cannot represent most
// decimal amounts exactly (0.1 + 0.2 != 0.3) and can silently drop digits
// on large values.
//
// It is shared because every service must parse, format and multiply money
// exactly the same way: a price read differently by two services is a money bug.
package money

import "errors"

type Currency string

const (
	BRL Currency = "BRL" // Brazilian Real
	VIB Currency = "VIB" // Vibranium | Fake currency
)

var decimalByCurrency = map[Currency]int{
	BRL: 2,
	VIB: 0,
}

var (
	ErrUnknownCurrency = errors.New("unknown currency")
	ErrInvalidFormat   = errors.New("invalid amount format")
	ErrTooManyDecimals = errors.New("too many decimals for currency")
	ErrNegative        = errors.New("negative amount")
	ErrOutOfRange      = errors.New("amount out of range")
	ErrOverflow        = errors.New("amount overflow")
	ErrZeroQuantity    = errors.New("zero quantity")
)

func (c Currency) Decimals() (int, error) {
	decimals, ok := decimalByCurrency[c]
	if !ok {
		return 0, ErrUnknownCurrency
	}

	return decimals, nil
}
