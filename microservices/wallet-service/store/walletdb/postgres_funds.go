package walletdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

// The rules live in apply_funds_batch (migration 20261006000000), so a batch
// is one round trip, and one statement applies all its operations or none.
const applyFundsBatch = `
SELECT operation_position, operation_result FROM apply_funds_batch($1) ORDER BY operation_position`

var errUnexpectedFundsResults = errors.New("apply funds batch: the results do not match the operations")

// One element of the batch apply_funds_batch reads. The movement id and date
// are made here, like those of every other movement, so the ledger stays in
// creation order by id.
type fundsRow struct {
	Position  int                 `json:"position"`
	ID        uuid.UUID           `json:"id"`
	Type      models.MovementType `json:"type"`
	MessageID uuid.UUID           `json:"message_id"`
	OrderID   uuid.UUID           `json:"order_id"`
	UserID    string              `json:"user_id"`
	Currency  money.Currency      `json:"currency"`
	Amount    int64               `json:"amount,string"`
	CreatedAt time.Time           `json:"created_at"`
}

func (postgres) applyFundsBatch(ctx *gofr.Context, operations []models.FundsOperation) ([]models.FundsResult, error) {
	encodedBatch, err := formatFundsBatch(operations)
	if err != nil {
		return nil, err
	}

	rows, err := ctx.SQL.QueryContext(ctx, applyFundsBatch, encodedBatch)
	if err != nil {
		return nil, fmt.Errorf("apply funds batch: %w", err)
	}
	defer rows.Close()

	return listFundsResults(rows, operations)
}

func formatFundsBatch(operations []models.FundsOperation) (string, error) {
	batch := make([]fundsRow, 0, len(operations))

	for position, operation := range operations {
		movementID, err := uuid.NewV7()
		if err != nil {
			return "", fmt.Errorf("generate movement id: %w", err)
		}

		batch = append(batch, fundsRow{
			Position: position, ID: movementID, Type: operation.Type, MessageID: operation.MessageID,
			OrderID: operation.OrderID, UserID: operation.UserID, Currency: operation.Currency,
			Amount: operation.Amount, CreatedAt: time.Now().UTC(),
		})
	}

	encodedBatch, err := json.Marshal(batch)
	if err != nil {
		return "", fmt.Errorf("encode funds batch: %w", err)
	}

	return string(encodedBatch), nil
}

func listFundsResults(rows *sql.Rows, operations []models.FundsOperation) ([]models.FundsResult, error) {
	results := make([]models.FundsResult, 0, len(operations))

	for rows.Next() {
		var (
			position int
			result   models.Result
		)

		if err := rows.Scan(&position, &result); err != nil {
			return nil, fmt.Errorf("scan funds result: %w", err)
		}

		if position != len(results) || position >= len(operations) {
			return nil, errUnexpectedFundsResults
		}

		operation := operations[position]
		results = append(results, models.FundsResult{MessageID: operation.MessageID, Type: operation.Type, Result: result})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("apply funds batch: %w", err)
	}

	if len(results) != len(operations) {
		return nil, errUnexpectedFundsResults
	}

	return results, nil
}
