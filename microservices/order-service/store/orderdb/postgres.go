package orderdb

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
)

const orderColumns = `id, user_id, book, side, type, limit_price, amount, quantity,
	filled_quantity, filled_amount, status, reason, created_at, updated_at`

// Filtering by user_id makes an order of someone else not found, so its existence is not revealed.
const getOrder = `
SELECT ` + orderColumns + `
FROM orders
WHERE id = $1 AND user_id = $2`

const insertOrderRow = `
INSERT INTO orders (` + orderColumns + `)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

const insertOrder = insertOrderRow + `
ON CONFLICT (id) DO NOTHING`

// Only the order's first event takes it out of PENDING.
const insertOrUpdateOrder = insertOrderRow + `
ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, reason = EXCLUDED.reason, updated_at = EXCLUDED.updated_at
WHERE orders.status = 'PENDING'`

const updateCancelledOrder = `
UPDATE orders SET status = 'CANCELLED', reason = $2, updated_at = $3
WHERE id = $1 AND status NOT IN ('FILLED', 'CANCELLED', 'REJECTED')`

// filled_quantity is read in the same statement, so a trade applied at the
// same time is counted.
const updateModifiedOrder = `
UPDATE orders SET limit_price = $2, quantity = filled_quantity + $3, updated_at = $4
WHERE id = $1 AND status NOT IN ('FILLED', 'CANCELLED', 'REJECTED')`

const insertTrade = `
INSERT INTO trades (trade_id, buy_order_id, sell_order_id, price, quantity, amount, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (trade_id) DO NOTHING`

// The right side of each SET reads the row before the update. A market buy is
// by amount, so it is filled when it spent its amount; any other order when it
// executed its quantity. Until then a limit order is PARTIALLY_FILLED and a
// market order keeps its status.
const addTradeToOrder = `
UPDATE orders SET
	filled_quantity = filled_quantity + $2,
	filled_amount = filled_amount + $3,
	status = CASE
		WHEN type = 'MARKET' AND side = 'BUY' AND filled_amount + $3 >= amount THEN 'FILLED'
		WHEN NOT (type = 'MARKET' AND side = 'BUY') AND filled_quantity + $2 >= quantity THEN 'FILLED'
		WHEN type = 'LIMIT' THEN 'PARTIALLY_FILLED'
		ELSE status
	END,
	updated_at = $4
WHERE id = $1 AND status NOT IN ('FILLED', 'CANCELLED', 'REJECTED')`

// A NULL parameter disables its filter. The id is a UUIDv7, so ORDER BY id
// is creation order and the orders_by_user (user_id, id DESC) index serves it.
const listOrders = `
SELECT ` + orderColumns + `
FROM orders
WHERE user_id = $1
	AND ($2::text IS NULL OR status = $2)
	AND ($3::text IS NULL OR side = $3)
	AND ($4::text IS NULL OR book = $4)
	AND ($5::uuid IS NULL OR id < $5)
ORDER BY id DESC
LIMIT $6`

func (postgres) insertOrder(ctx *gofr.Context, order models.Order) error {
	_, err := ctx.SQL.ExecContext(ctx, insertOrder,
		order.ID, order.UserID, order.Book, order.Side, order.Type, order.Limit, order.Amount, order.Quantity,
		order.FilledQuantity, order.FilledAmount, order.Status, order.Reason, order.CreatedAt, order.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	return nil
}

func (postgres) insertOrUpdateOrder(ctx *gofr.Context, order models.Order) error {
	_, err := ctx.SQL.ExecContext(ctx, insertOrUpdateOrder,
		order.ID, order.UserID, order.Book, order.Side, order.Type, order.Limit, order.Amount, order.Quantity,
		order.FilledQuantity, order.FilledAmount, order.Status, order.Reason, order.CreatedAt, order.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert or update order: %w", err)
	}

	return nil
}

func (postgres) updateCancelledOrder(ctx *gofr.Context, id uuid.UUID, reason *string, cancelledAt time.Time) error {
	if _, err := ctx.SQL.ExecContext(ctx, updateCancelledOrder, id, reason, cancelledAt); err != nil {
		return fmt.Errorf("update cancelled order: %w", err)
	}

	return nil
}

func (postgres) updateModifiedOrder(ctx *gofr.Context, id uuid.UUID, limit, pendingQuantity int64, modifiedAt time.Time) error {
	if _, err := ctx.SQL.ExecContext(ctx, updateModifiedOrder, id, limit, pendingQuantity, modifiedAt); err != nil {
		return fmt.Errorf("update modified order: %w", err)
	}

	return nil
}

func (postgres) insertTrade(ctx *gofr.Context, trade models.Trade) error {
	tx, err := ctx.SQL.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	if err := addTradeToOrders(ctx, tx, trade); err != nil {
		_ = tx.Rollback()

		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// A trade already in trades affects no row, so its orders are left as they are.
func addTradeToOrders(ctx *gofr.Context, tx *gofrSQL.Tx, trade models.Trade) error {
	result, err := tx.ExecContext(ctx, insertTrade, trade.ID, trade.BuyOrderID, trade.SellOrderID, trade.Price,
		trade.Quantity, trade.Amount, trade.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert trade: %w", err)
	}

	if rows, err := result.RowsAffected(); err != nil || rows == 0 {
		return err
	}

	for _, orderID := range []uuid.UUID{trade.BuyOrderID, trade.SellOrderID} {
		if _, err := tx.ExecContext(ctx, addTradeToOrder, orderID, trade.Quantity, trade.Amount, trade.CreatedAt); err != nil {
			return fmt.Errorf("add trade to order: %w", err)
		}
	}

	return nil
}

func (postgres) listOrders(ctx *gofr.Context, query ListQuery) ([]models.Order, error) {
	rows, err := ctx.SQL.QueryContext(ctx, listOrders, query.UserID, query.Status, query.Side, query.Book, query.Cursor, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	orders := make([]models.Order, 0, query.Limit)

	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}

	return orders, nil
}

func (postgres) getOrder(ctx *gofr.Context, userID string, id uuid.UUID) (models.Order, error) {
	order, err := scanOrder(ctx.SQL.QueryRowContext(ctx, getOrder, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Order{}, models.ErrOrderNotFound
	}

	if err != nil {
		return models.Order{}, fmt.Errorf("get order: %w", err)
	}

	return order, nil
}

// Satisfied by *sql.Row and *sql.Rows; reads the columns of orderColumns, in that order.
func scanOrder(row interface{ Scan(dest ...any) error }) (models.Order, error) {
	var (
		order                   models.Order
		limit, amount, quantity sql.NullInt64
		reason                  sql.NullString
	)

	err := row.Scan(&order.ID, &order.UserID, &order.Book, &order.Side, &order.Type, &limit, &amount,
		&quantity, &order.FilledQuantity, &order.FilledAmount, &order.Status, &reason, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return models.Order{}, fmt.Errorf("scan order: %w", err)
	}

	order.Limit = parseNullInt64(limit)
	order.Amount = parseNullInt64(amount)
	order.Quantity = parseNullInt64(quantity)

	if reason.Valid {
		order.Reason = &reason.String
	}

	return order, nil
}

func parseNullInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}

	return &value.Int64
}
