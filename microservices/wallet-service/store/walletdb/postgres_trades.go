package walletdb

import (
	"fmt"

	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
)

const isTradeApplied = `
SELECT EXISTS (SELECT 1 FROM ledger WHERE type = 'TRADE_PAID' AND message_id = $1)`

// The engine reserved the trade before it crossed; CHECK (reserved >= 0)
// fails the transaction if that ever were not so.
const takeReserved = `
UPDATE balances SET reserved = reserved - $3
WHERE user_id = $1 AND currency = $2`

func (postgres) applyTrade(ctx *gofr.Context, trade models.Trade) error {
	movements, err := trade.BuildMovements()
	if err != nil {
		return err
	}

	return runInTransaction(ctx, func(tx *gofrSQL.Tx) error {
		var applied bool
		if err := tx.QueryRowContext(ctx, isTradeApplied, trade.MessageID).Scan(&applied); err != nil || applied {
			return err
		}

		for _, movement := range movements {
			if err := applyTradeMovement(ctx, tx, movement); err != nil {
				return err
			}
		}

		return nil
	})
}

func applyTradeMovement(ctx *gofr.Context, tx *gofrSQL.Tx, movement models.Movement) error {
	query := addAvailable
	if movement.Type == models.MovementTradePaid {
		query = takeReserved
	}

	if _, err := tx.ExecContext(ctx, query, movement.UserID, movement.Currency, movement.Amount); err != nil {
		return fmt.Errorf("apply %s: %w", movement.Type, err)
	}

	return insertMovementIn(ctx, tx, movement)
}
