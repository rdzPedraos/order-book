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

	return getInt64(cost)
}

// How much of base a market buy gets for amount at price: the opposite of
// Notional. Example: R$ 500.00 (amount 50000 cents) at R$ 110.00 (price 11000)
// buys 4 VIB. A fraction of the base's smallest unit rounds down, so the buy
// never takes more than its money covers.
func QuantityFor(base Currency, amount, price int64) (int64, error) {
	if amount < 0 {
		return 0, ErrNegative
	}

	if price <= 0 {
		return 0, ErrOutOfRange
	}

	decimals, err := base.GetDecimals()
	if err != nil {
		return 0, err
	}

	quantity := new(big.Int).Mul(big.NewInt(amount), big.NewInt(pow10(decimals)))
	quantity.Quo(quantity, big.NewInt(price))

	return getInt64(quantity)
}

// The average price an order paid: total spent over the quantity executed.
// Example: R$ 410.00 (total 41000) for 4 VIB averages R$ 102.50 (10250). Half a
// cent rounds up. It is informative only: no balance is computed from it.
func AveragePrice(base Currency, total, quantity int64) (int64, error) {
	if total < 0 || quantity < 0 {
		return 0, ErrNegative
	}

	if quantity == 0 {
		return 0, nil
	}

	decimals, err := base.GetDecimals()
	if err != nil {
		return 0, err
	}

	average := new(big.Int).Mul(big.NewInt(total), big.NewInt(pow10(decimals)))
	average, leftover := average.QuoRem(average, big.NewInt(quantity), new(big.Int))

	// The leftover is at least half the quantity when the dropped fraction is ≥ 0.5.
	if leftover.Mul(leftover, big.NewInt(2)).Cmp(big.NewInt(quantity)) >= 0 {
		average.Add(average, big.NewInt(1))
	}

	return getInt64(average)
}

func getInt64(value *big.Int) (int64, error) {
	if !value.IsInt64() {
		return 0, ErrOverflow
	}

	return value.Int64(), nil
}
