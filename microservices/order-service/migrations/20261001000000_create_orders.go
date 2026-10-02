package migrations

import "gofr.dev/pkg/gofr/migration"

// Amounts are BIGINT in minimal units: price and amount in the quote
// currency, quantity and filled_quantity in the base asset.
const createOrdersTable = `
CREATE TABLE IF NOT EXISTS orders (
	id              UUID        PRIMARY KEY,
	user_id         TEXT        NOT NULL,
	book            TEXT        NOT NULL,
	side            TEXT        NOT NULL CHECK (side IN ('BUY', 'SELL')),
	type            TEXT        NOT NULL CHECK (type IN ('LIMIT', 'MARKET')),
	limit_price     BIGINT,
	amount          BIGINT,
	quantity        BIGINT,
	filled_quantity BIGINT      NOT NULL DEFAULT 0,
	avg_price       BIGINT,
	status          TEXT        NOT NULL,
	created_at      TIMESTAMPTZ NOT NULL,
	updated_at      TIMESTAMPTZ NOT NULL
)`

// The id is a UUIDv7, so ordering by id is ordering by creation time.
const createOrdersByUserIndex = `
CREATE INDEX IF NOT EXISTS orders_by_user ON orders (user_id, id DESC)`

func createOrders() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			if _, err := d.SQL.Exec(createOrdersTable); err != nil {
				return err
			}

			_, err := d.SQL.Exec(createOrdersByUserIndex)

			return err
		},
	}
}
