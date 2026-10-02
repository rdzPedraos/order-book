// Package walletdb is the wallet-service's access to its PostgreSQL database:
// balances and their ledger. Handlers call its functions directly; tests
// replace the database with InitMock.
package walletdb

import (
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
)

// A nil filter does not filter. To is exclusive; Cursor continues after that movement.
type MovementQuery struct {
	UserID string
	From   *time.Time
	To     *time.Time
	Cursor *uuid.UUID
	Limit  int
}

type walletDB interface {
	getBalances(ctx *gofr.Context, userID string) ([]models.Balance, error)
	deposit(ctx *gofr.Context, movement models.Movement) error
	withdraw(ctx *gofr.Context, movement models.Movement) error
	listMovements(ctx *gofr.Context, query MovementQuery) ([]models.Movement, error)
	applyFundsBatch(ctx *gofr.Context, operations []models.FundsOperation) ([]models.FundsResult, error)
}

type postgres struct{}

var db walletDB = postgres{}

// Every wallet currency is in the answer; one without a row has a zero balance.
func GetBalances(ctx *gofr.Context, userID string) ([]models.Balance, error) {
	stored, err := db.getBalances(ctx, userID)
	if err != nil {
		return nil, err
	}

	return fillWalletCurrencies(userID, stored), nil
}

// Adds the amount to available and writes the movement, in one transaction.
func Deposit(ctx *gofr.Context, movement models.Movement) error {
	return db.deposit(ctx, movement)
}

// Takes the amount from available only, never from reserved, and writes the
// movement, in one transaction; models.ErrInsufficientFunds when it is short.
func Withdraw(ctx *gofr.Context, movement models.Movement) error {
	return db.withdraw(ctx, movement)
}

// Only the movements that moved money: a rejected reservation is in the ledger
// but is not listed. Newest first.
func ListMovements(ctx *gofr.Context, query MovementQuery) ([]models.Movement, error) {
	return db.listMovements(ctx, query)
}

// Applies the operations in order, in one transaction, and answers each one's
// result. A message already applied answers its stored result without moving
// funds again, including a rejected reservation.
func ApplyFundsBatch(ctx *gofr.Context, operations []models.FundsOperation) ([]models.FundsResult, error) {
	return db.applyFundsBatch(ctx, operations)
}

func fillWalletCurrencies(userID string, stored []models.Balance) []models.Balance {
	balances := make([]models.Balance, 0, len(models.WalletCurrencies))

	for _, currency := range models.WalletCurrencies {
		balance := models.Balance{UserID: userID, Currency: currency}

		for _, row := range stored {
			if row.Currency == currency {
				balance = row
			}
		}

		balances = append(balances, balance)
	}

	return balances
}
