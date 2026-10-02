// Package migrations is the schema of the market_service database, applied by
// Gofr on startup in version order.
package migrations

import "gofr.dev/pkg/gofr/migration"

func All() map[int64]migration.Migrate {
	return map[int64]migration.Migrate{
		20261005000000: createLevels(),
	}
}
