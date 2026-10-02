package migrations

import "gofr.dev/pkg/gofr/migration"

// Amounts are BIGINT in the currency's minimal units. A person without a row
// for a currency has a zero balance in it; the first deposit creates the row.
const createBalancesTable = `
CREATE TABLE IF NOT EXISTS balances (
	user_id   TEXT   NOT NULL,
	currency  TEXT   NOT NULL,
	available BIGINT NOT NULL DEFAULT 0 CHECK (available >= 0),
	reserved  BIGINT NOT NULL DEFAULT 0 CHECK (reserved >= 0),
	PRIMARY KEY (user_id, currency)
)`

func createBalances() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createBalancesTable)

			return err
		},
	}
}
