package money

import "math/big"

// How much it costs to buy quantity of base at price: what a limit buy must
// freeze. Example: 10 VIB at R$ 90.00 (price 9000 cents) costs 90000 cents.
//
// If the base has decimals, quantity comes in its smallest unit, so the result
// is divided by them: 0.50 of a base with 2 decimals (quantity 50) at 9000 is
// 50 × 9000 / 100 = 4500. A cent left over rounds up, never down, so the order
// never runs short of money.
func Notional(base Currency, quantity, price int64) (int64, error) {
	if quantity < 0 || price < 0 {
		return 0, ErrNegative
	}

	decimals, err := base.GetDecimals()
	if err != nil {
		return 0, err
	}

	// A big integer has no size limit, so quantity × price never overflows here
	// even when it is larger than an int64; only the final result must fit.
	cost := new(big.Int).Mul(big.NewInt(quantity), big.NewInt(price))
	cost, leftover := cost.QuoRem(cost, big.NewInt(pow10(decimals)), new(big.Int))

	if leftover.Sign() > 0 {
		cost.Add(cost, big.NewInt(1))
	}

	if !cost.IsInt64() {
		return 0, ErrOverflow
	}

	return cost.Int64(), nil
}
