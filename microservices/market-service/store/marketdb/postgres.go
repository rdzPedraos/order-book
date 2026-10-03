package marketdb

import (
	"encoding/json"
	"fmt"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
)

// Each level appears once, so the upsert never touches a row twice, and goes
// to the delete when it emptied or to the upsert otherwise.
const updateLevels = `
WITH changes AS (
	SELECT * FROM jsonb_to_recordset($1) AS c(book text, side text, price bigint, volume bigint, orders int)
),
emptied AS (
	DELETE FROM levels USING changes
	WHERE changes.volume = 0 AND levels.book = changes.book AND levels.side = changes.side AND levels.price = changes.price
)
INSERT INTO levels (book, side, price, volume, orders)
SELECT book, side, price, volume, orders FROM changes WHERE volume > 0
ON CONFLICT (book, side, price) DO UPDATE SET volume = EXCLUDED.volume, orders = EXCLUDED.orders`

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

type levelRow struct {
	Book   string `json:"book"`
	Side   string `json:"side"`
	Price  int64  `json:"price,string"`
	Volume int64  `json:"volume,string"`
	Orders int    `json:"orders"`
}

func (postgres) updateLevels(ctx *gofr.Context, levels []models.Level) error {
	encodedLevels, err := json.Marshal(buildLastLevelRows(levels))
	if err != nil {
		return fmt.Errorf("encode levels: %w", err)
	}

	if _, err := ctx.SQL.ExecContext(ctx, updateLevels, string(encodedLevels)); err != nil {
		return fmt.Errorf("update levels: %w", err)
	}

	return nil
}

// Each event carries the whole state of its level, so only the last one of
// each level in the batch matters.
func buildLastLevelRows(levels []models.Level) []levelRow {
	rows := make([]levelRow, 0, len(levels))
	positions := make(map[levelRow]int, len(levels))

	for _, level := range levels {
		key := levelRow{Book: level.Book, Side: level.Side, Price: level.Price}
		row := levelRow{Book: level.Book, Side: level.Side, Price: level.Price, Volume: level.Volume, Orders: level.Orders}

		if position, seen := positions[key]; seen {
			rows[position] = row

			continue
		}

		positions[key] = len(rows)
		rows = append(rows, row)
	}

	return rows
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
