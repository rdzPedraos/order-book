package migrations

import "gofr.dev/pkg/gofr/migration"

// avg_price was never written, so it becomes filled_amount, what the order's
// trades paid or received; the average price is computed from it when the
// order is read. trades records each trade once, so a repeated TradeExecuted
// is not added to its orders twice.
var trackExecutionsStatements = []string{
	`ALTER TABLE orders RENAME COLUMN avg_price TO filled_amount`,
	`UPDATE orders SET filled_amount = 0 WHERE filled_amount IS NULL`,
	`ALTER TABLE orders ALTER COLUMN filled_amount SET DEFAULT 0, ALTER COLUMN filled_amount SET NOT NULL`,
	`CREATE TABLE IF NOT EXISTS trades (
		trade_id      UUID        PRIMARY KEY,
		buy_order_id  UUID        NOT NULL,
		sell_order_id UUID        NOT NULL,
		price         BIGINT      NOT NULL,
		quantity      BIGINT      NOT NULL,
		amount        BIGINT      NOT NULL,
		created_at    TIMESTAMPTZ NOT NULL
	)`,
}

func trackExecutions() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			for _, statement := range trackExecutionsStatements {
				if _, err := d.SQL.Exec(statement); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
