// Package migrations is the schema of the order_service database, applied by
// Gofr on startup in version order.
package migrations

import "gofr.dev/pkg/gofr/migration"

func All() map[int64]migration.Migrate {
	return map[int64]migration.Migrate{
		20261001000000: createOrders(),
		20261005000000: addReason(),
		20261005000001: trackExecutions(),
	}
}
