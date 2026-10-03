package walletdb

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

// The rules live in apply_trade_movements (migration 20261007000000), so a
// batch is one round trip, and one statement applies all its trades or none.
const applyTradeMovements = `SELECT apply_trade_movements($1)`

// The keys are the columns of ledger, which the function reads with
// jsonb_populate_recordset.
type movementRow struct {
	ID        uuid.UUID           `json:"id"`
	UserID    string              `json:"user_id"`
	Currency  money.Currency      `json:"currency"`
	Type      models.MovementType `json:"type"`
	Amount    int64               `json:"amount,string"`
	OrderID   *uuid.UUID          `json:"order_id"`
	MessageID uuid.UUID           `json:"message_id"`
	Result    models.Result       `json:"result"`
	CreatedAt time.Time           `json:"created_at"`
}

func (postgres) applyTrades(ctx *gofr.Context, trades []models.Trade) error {
	encodedMovements, err := formatTradeMovements(trades)
	if err != nil {
		return err
	}

	if _, err := ctx.SQL.ExecContext(ctx, applyTradeMovements, encodedMovements); err != nil {
		return fmt.Errorf("apply trades: %w", err)
	}

	return nil
}

func formatTradeMovements(trades []models.Trade) (string, error) {
	rows := make([]movementRow, 0, 4*len(trades))

	for _, trade := range trades {
		movements, err := trade.BuildMovements()
		if err != nil {
			return "", err
		}

		for _, movement := range movements {
			rows = append(rows, movementRow{
				ID: movement.ID, UserID: movement.UserID, Currency: movement.Currency, Type: movement.Type,
				Amount: movement.Amount, OrderID: movement.OrderID, MessageID: *movement.MessageID,
				Result: movement.Result, CreatedAt: movement.CreatedAt,
			})
		}
	}

	encodedMovements, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("encode trade movements: %w", err)
	}

	return string(encodedMovements), nil
}
