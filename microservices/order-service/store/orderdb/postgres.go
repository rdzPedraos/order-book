package orderdb

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"gofr.dev/pkg/gofr"
)

const orderColumns = `id, user_id, book, side, type, limit_price, amount, quantity,
	filled_quantity, avg_price, status, created_at, updated_at`

// Filtering by user_id makes an order of someone else not found, so its existence is not revealed.
const getOrder = `
SELECT ` + orderColumns + `
FROM orders
WHERE id = $1 AND user_id = $2`

const insertOrder = `
INSERT INTO orders (` + orderColumns + `)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (id) DO NOTHING`

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
		order.FilledQuantity, order.AvgPrice, order.Status, order.CreatedAt, order.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
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
		order                             models.Order
		limit, amount, quantity, avgPrice sql.NullInt64
	)

	err := row.Scan(&order.ID, &order.UserID, &order.Book, &order.Side, &order.Type, &limit, &amount,
		&quantity, &order.FilledQuantity, &avgPrice, &order.Status, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return models.Order{}, fmt.Errorf("scan order: %w", err)
	}

	order.Limit = parseNullInt64(limit)
	order.Amount = parseNullInt64(amount)
	order.Quantity = parseNullInt64(quantity)
	order.AvgPrice = parseNullInt64(avgPrice)

	return order, nil
}

func parseNullInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}

	return &value.Int64
}
