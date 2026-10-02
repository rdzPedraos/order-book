package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/shared/money"
)

type MovementType string

const (
	MovementDeposit    MovementType = "DEPOSIT"
	MovementWithdrawal MovementType = "WITHDRAWAL"
	MovementReserve    MovementType = "RESERVE"
	MovementRelease    MovementType = "RELEASE"
)

type Result string

const (
	ResultOK                Result = "OK"
	ResultInsufficientFunds Result = "insufficient_funds"
)

// One row of the ledger. OrderID and MessageID are nil for deposits and
// withdrawals, which do not come from the log.
type Movement struct {
	ID        uuid.UUID
	UserID    string
	Currency  money.Currency
	Type      MovementType
	Amount    int64
	OrderID   *uuid.UUID
	MessageID *uuid.UUID
	Result    Result
	CreatedAt time.Time
}

// Builds a deposit or a withdrawal from the request's currency and amount,
// which travels as a decimal string in the currency's scale.
func NewMovement(movementType MovementType, userID, currency string, amount *string) (*Movement, error) {
	walletCurrency := money.Currency(strings.ToUpper(currency))
	if !IsWalletCurrency(walletCurrency) {
		return nil, ErrInvalidCurrency
	}

	parsedAmount, err := money.Parse(walletCurrency, amount)
	if err != nil || parsedAmount == nil {
		return nil, ErrInvalidAmount
	}

	movementID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate movement id: %w", err)
	}

	movement := Movement{
		ID: movementID, UserID: userID, Currency: walletCurrency, Type: movementType, Amount: *parsedAmount,
		Result: ResultOK, CreatedAt: time.Now().UTC(),
	}

	if err := movement.Validate(); err != nil {
		return nil, err
	}

	return &movement, nil
}

func (m Movement) Validate() error {
	if !IsWalletCurrency(m.Currency) {
		return ErrInvalidCurrency
	}

	if m.Type == MovementWithdrawal && m.Currency != money.BRL {
		return ErrCurrencyNotWithdrawable
	}

	if m.Amount <= 0 {
		return ErrInvalidAmount
	}

	return nil
}

func IsWalletCurrency(currency money.Currency) bool {
	for _, walletCurrency := range WalletCurrencies {
		if currency == walletCurrency {
			return true
		}
	}

	return false
}
