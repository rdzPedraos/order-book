package walletdb

import (
	"fmt"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
)

// A NULL parameter disables its filter. The id is a UUIDv7, so ORDER BY id is
// creation order and the ledger_by_user (user_id, id DESC) index serves it.
const listMovements = `
SELECT id, user_id, currency, type, amount, order_id, message_id, result, created_at
FROM ledger
WHERE user_id = $1
	AND result = 'OK'
	AND ($2::timestamptz IS NULL OR created_at >= $2)
	AND ($3::timestamptz IS NULL OR created_at < $3)
	AND ($4::uuid IS NULL OR id < $4)
ORDER BY id DESC
LIMIT $5`

func (postgres) listMovements(ctx *gofr.Context, query MovementQuery) ([]models.Movement, error) {
	rows, err := ctx.SQL.QueryContext(ctx, listMovements, query.UserID, query.From, query.To, query.Cursor, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}
	defer rows.Close()

	movements := make([]models.Movement, 0, query.Limit)

	for rows.Next() {
		movement, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}

		movements = append(movements, movement)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}

	return movements, nil
}

func scanMovement(row interface{ Scan(dest ...any) error }) (models.Movement, error) {
	var (
		movement           models.Movement
		orderID, messageID uuid.NullUUID
	)

	err := row.Scan(&movement.ID, &movement.UserID, &movement.Currency, &movement.Type, &movement.Amount,
		&orderID, &messageID, &movement.Result, &movement.CreatedAt)
	if err != nil {
		return models.Movement{}, fmt.Errorf("scan movement: %w", err)
	}

	movement.OrderID = parseNullUUID(orderID)
	movement.MessageID = parseNullUUID(messageID)

	return movement, nil
}

func parseNullUUID(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}

	return &value.UUID
}
