package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Never goes through float64.
func Parse(c Currency, value *string) (*int64, error) {
	if value == nil {
		return nil, nil
	}

	input := *value

	decimals, err := c.Decimals()
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(input, "-") {
		return nil, ErrNegative
	}

	whole, fraction, err := splitDecimal(input)
	if err != nil {
		return nil, err
	}

	if len(fraction) > decimals {
		return nil, ErrTooManyDecimals
	}

	digits := whole + fraction + strings.Repeat("0", decimals-len(fraction))

	amount, err := strconv.ParseInt(digits, 10, 64)
	if errors.Is(err, strconv.ErrRange) {
		return nil, ErrOutOfRange
	}

	if err != nil {
		return nil, ErrInvalidFormat
	}

	return &amount, nil
}

func Format(c Currency, amount int64) (string, error) {
	decimals, err := c.Decimals()
	if err != nil {
		return "", err
	}

	if amount < 0 {
		return "", ErrNegative
	}

	if decimals == 0 {
		return strconv.FormatInt(amount, 10), nil
	}

	scale := pow10(decimals)

	return fmt.Sprintf("%d.%0*d", amount/scale, decimals, amount%scale), nil
}

func splitDecimal(input string) (whole, fraction string, err error) {
	whole, fraction, hasDot := strings.Cut(input, ".")

	if !isDigits(whole) {
		return "", "", ErrInvalidFormat
	}

	if hasDot && !isDigits(fraction) {
		return "", "", ErrInvalidFormat
	}

	return whole, fraction, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

func pow10(n int) int64 {
	result := int64(1)
	for range n {
		result *= 10
	}

	return result
}
