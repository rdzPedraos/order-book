package models

import (
	"net/http"

	"github.com/rdzpedraos/order-book/shared/fault"
)

var (
	ErrInvalidAmount   = fault.New("invalid_amount", "amount must be a positive amount of the currency")
	ErrInvalidCurrency = fault.New("invalid_currency", "currency must be BRL or VIB")
)

var (
	ErrInsufficientFunds       = fault.NewWithStatus(http.StatusUnprocessableEntity, "insufficient_funds", "not enough available balance")
	ErrCurrencyNotWithdrawable = fault.NewWithStatus(http.StatusUnprocessableEntity, "currency_not_withdrawable", "only BRL can be withdrawn")
)
