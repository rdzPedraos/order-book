package migrations

import "gofr.dev/pkg/gofr/migration"

// The engine's reason for a rejected or cancelled order.
const addOrderReason = `ALTER TABLE orders ADD COLUMN IF NOT EXISTS reason TEXT`

func addReason() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(addOrderReason)

			return err
		},
	}
}
