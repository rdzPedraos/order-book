// Package migrations is the schema of the wallet_service database, applied by
// Gofr on startup in version order.
package migrations

import "gofr.dev/pkg/gofr/migration"

func All() map[int64]migration.Migrate {
	return map[int64]migration.Migrate{
		20261003000000: createBalances(),
		20261003000001: createLedger(),
		20261005000000: addTradeMovements(),
		20261006000000: createApplyFundsBatch(),
		20261007000000: createApplyTradeMovements(),
		20261008000000: lockBalancesInOrder(),
	}
}
