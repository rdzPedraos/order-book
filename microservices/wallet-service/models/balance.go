package models

import "github.com/rdzpedraos/order-book/shared/money"

// Every wallet holds these currencies, in this order, even before its first deposit.
var WalletCurrencies = []money.Currency{money.BRL, money.VIB}

// Amounts are in the currency's minimal units.
type Balance struct {
	UserID    string
	Currency  money.Currency
	Available int64
	Reserved  int64
}

func (b Balance) GetTotal() int64 {
	return b.Available + b.Reserved
}
