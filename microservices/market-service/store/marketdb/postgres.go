package marketdb

import (
	"fmt"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
)

const upsertLevel = `
INSERT INTO levels (book, side, price, volume, orders)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (book, side, price) DO UPDATE SET volume = EXCLUDED.volume, orders = EXCLUDED.orders`

const deleteLevel = `
DELETE FROM levels WHERE book = $1 AND side = $2 AND price = $3`

const listBids = `
SELECT price, volume, orders FROM levels
WHERE book = $1 AND side = 'BUY'
ORDER BY price DESC
LIMIT $2`

const listAsks = `
SELECT price, volume, orders FROM levels
WHERE book = $1 AND side = 'SELL'
ORDER BY price ASC
LIMIT $2`

func (postgres) updateLevel(ctx *gofr.Context, level models.Level) error {
	var err error
	if level.Volume == 0 {
		_, err = ctx.SQL.ExecContext(ctx, deleteLevel, level.Book, level.Side, level.Price)
	} else {
		_, err = ctx.SQL.ExecContext(ctx, upsertLevel, level.Book, level.Side, level.Price, level.Volume, level.Orders)
	}

	if err != nil {
		return fmt.Errorf("update level: %w", err)
	}

	return nil
}

func (postgres) listLevels(ctx *gofr.Context, book, side string, depth int) ([]models.Level, error) {
	query := listAsks
	if side == "BUY" {
		query = listBids
	}

	rows, err := ctx.SQL.QueryContext(ctx, query, book, depth)
	if err != nil {
		return nil, fmt.Errorf("list levels: %w", err)
	}
	defer rows.Close()

	levels := make([]models.Level, 0, depth)

	for rows.Next() {
		level := models.Level{Book: book, Side: side}
		if err := rows.Scan(&level.Price, &level.Volume, &level.Orders); err != nil {
			return nil, fmt.Errorf("scan level: %w", err)
		}

		levels = append(levels, level)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list levels: %w", err)
	}

	return levels, nil
}
