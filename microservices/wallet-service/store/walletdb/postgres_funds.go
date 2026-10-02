package walletdb

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
)

const findResult = `
SELECT result FROM ledger WHERE type = $1 AND message_id = $2`

// Like takeAvailable, a short balance affects no row instead of going negative.
const reserveFunds = `
UPDATE balances SET available = available - $3, reserved = reserved + $3
WHERE user_id = $1 AND currency = $2 AND available >= $3`

const releaseFunds = `
UPDATE balances SET reserved = reserved - $3, available = available + $3
WHERE user_id = $1 AND currency = $2 AND reserved >= $3`

func (postgres) applyFundsBatch(ctx *gofr.Context, operations []models.FundsOperation) ([]models.FundsResult, error) {
	results := make([]models.FundsResult, 0, len(operations))

	err := runInTransaction(ctx, func(tx *gofrSQL.Tx) error {
		for _, operation := range operations {
			result, err := applyFundsOperation(ctx, tx, operation)
			if err != nil {
				return err
			}

			results = append(results, models.FundsResult{MessageID: operation.MessageID, Type: operation.Type, Result: result})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}

func applyFundsOperation(ctx *gofr.Context, tx *gofrSQL.Tx, operation models.FundsOperation) (models.Result, error) {
	stored, err := findStoredResult(ctx, tx, operation)
	if err != nil || stored != "" {
		return stored, err
	}

	if operation.Type == models.MovementReserve {
		return reserve(ctx, tx, operation)
	}

	return release(ctx, tx, operation)
}

// Empty when the message was never applied.
func findStoredResult(ctx *gofr.Context, tx *gofrSQL.Tx, operation models.FundsOperation) (models.Result, error) {
	var stored models.Result

	err := tx.QueryRowContext(ctx, findResult, operation.Type, operation.MessageID).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("find result: %w", err)
	}

	return stored, nil
}

// A rejected reservation is recorded too, so a repeated message gets the same
// answer even if the person deposited in between.
func reserve(ctx *gofr.Context, tx *gofrSQL.Tx, operation models.FundsOperation) (models.Result, error) {
	moved, err := updateBalance(ctx, tx, reserveFunds, operation)
	if err != nil {
		return "", fmt.Errorf("reserve funds: %w", err)
	}

	result := models.ResultOK
	if !moved {
		result = models.ResultInsufficientFunds
	}

	return result, insertFundsMovement(ctx, tx, operation, result)
}

func release(ctx *gofr.Context, tx *gofrSQL.Tx, operation models.FundsOperation) (models.Result, error) {
	moved, err := updateBalance(ctx, tx, releaseFunds, operation)
	if err != nil {
		return "", fmt.Errorf("release funds: %w", err)
	}

	if !moved {
		return models.ResultReleaseExceedsReservation, nil
	}

	return models.ResultOK, insertFundsMovement(ctx, tx, operation, models.ResultOK)
}

func updateBalance(ctx *gofr.Context, tx *gofrSQL.Tx, query string, operation models.FundsOperation) (bool, error) {
	result, err := tx.ExecContext(ctx, query, operation.UserID, operation.Currency, operation.Amount)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()

	return rows > 0, err
}

func insertFundsMovement(ctx *gofr.Context, tx *gofrSQL.Tx, operation models.FundsOperation, result models.Result) error {
	movementID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate movement id: %w", err)
	}

	return insertMovementIn(ctx, tx, models.Movement{
		ID: movementID, UserID: operation.UserID, Currency: operation.Currency, Type: operation.Type,
		Amount: operation.Amount, OrderID: &operation.OrderID, MessageID: &operation.MessageID,
		Result: result, CreatedAt: time.Now().UTC(),
	})
}
