package migrations

import "gofr.dev/pkg/gofr/migration"

// A trade writes four rows with the same message_id, the trade's event: each
// person pays in one currency and receives in the other. Adding currency to
// the key lets the four rows in and still keeps a repeated trade out.
var addTradeMovementsStatements = []string{
	`ALTER TABLE ledger DROP CONSTRAINT ledger_type_message_id_key`,
	`ALTER TABLE ledger ADD CONSTRAINT ledger_type_message_id_currency_key UNIQUE (type, message_id, currency)`,
	`ALTER TABLE ledger DROP CONSTRAINT ledger_type_check`,
	`ALTER TABLE ledger ADD CONSTRAINT ledger_type_check
		CHECK (type IN ('DEPOSIT', 'WITHDRAWAL', 'RESERVE', 'RELEASE', 'TRADE_PAID', 'TRADE_RECEIVED'))`,
}

func addTradeMovements() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			for _, statement := range addTradeMovementsStatements {
				if _, err := d.SQL.Exec(statement); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
