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

import (
	"net/http"

	"github.com/rdzpedraos/order-book/shared/fault"
)

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
	ErrUnknownCurrency = fault.NewWithStatus(http.StatusInternalServerError, "unknown_currency", "unknown currency")
	ErrInvalidFormat   = fault.New("invalid_amount_format", "invalid amount format")
	ErrTooManyDecimals = fault.New("too_many_decimals", "too many decimals for currency")
	ErrNegative        = fault.New("negative_amount", "negative amount")
	ErrOutOfRange      = fault.New("amount_out_of_range", "amount out of range")
)

func (c Currency) Decimals() (int, error) {
	decimals, ok := decimalByCurrency[c]
	if !ok {
		return 0, ErrUnknownCurrency
	}

	return decimals, nil
}
