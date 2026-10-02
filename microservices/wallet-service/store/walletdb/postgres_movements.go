package walletdb

import (
	"fmt"

	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
)

// The first deposit of a currency creates its row.
const addAvailable = `
INSERT INTO balances (user_id, currency, available)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, currency) DO UPDATE SET available = balances.available + EXCLUDED.available`

// Affects no row when available is short, so two withdrawals or a withdrawal
// and a reservation never spend the same balance: PostgreSQL locks the row and
// re-evaluates the WHERE after the other one commits.
const takeAvailable = `
UPDATE balances SET available = available - $3
WHERE user_id = $1 AND currency = $2 AND available >= $3`

const insertMovement = `
INSERT INTO ledger (id, user_id, currency, type, amount, order_id, message_id, result, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

func (postgres) deposit(ctx *gofr.Context, movement models.Movement) error {
	return runInTransaction(ctx, func(tx *gofrSQL.Tx) error {
		if _, err := tx.ExecContext(ctx, addAvailable, movement.UserID, movement.Currency, movement.Amount); err != nil {
			return fmt.Errorf("add available: %w", err)
		}

		return insertMovementIn(ctx, tx, movement)
	})
}

func (postgres) withdraw(ctx *gofr.Context, movement models.Movement) error {
	return runInTransaction(ctx, func(tx *gofrSQL.Tx) error {
		result, err := tx.ExecContext(ctx, takeAvailable, movement.UserID, movement.Currency, movement.Amount)
		if err != nil {
			return fmt.Errorf("take available: %w", err)
		}

		if rows, err := result.RowsAffected(); err != nil || rows == 0 {
			return models.ErrInsufficientFunds
		}

		return insertMovementIn(ctx, tx, movement)
	})
}

func insertMovementIn(ctx *gofr.Context, tx *gofrSQL.Tx, movement models.Movement) error {
	_, err := tx.ExecContext(ctx, insertMovement, movement.ID, movement.UserID, movement.Currency, movement.Type,
		movement.Amount, movement.OrderID, movement.MessageID, movement.Result, movement.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert movement: %w", err)
	}

	return nil
}
