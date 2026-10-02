package migrations

import "gofr.dev/pkg/gofr/migration"

// Rows are only inserted, never updated. message_id is the log message that
// caused a reservation or a release; UNIQUE (type, message_id) makes a repeated
// message apply once. It is NULL for deposits and withdrawals, which do not
// come from the log, and NULLs never conflict.
const createLedgerTable = `
CREATE TABLE IF NOT EXISTS ledger (
	id         UUID        PRIMARY KEY,
	user_id    TEXT        NOT NULL,
	currency   TEXT        NOT NULL,
	type       TEXT        NOT NULL CHECK (type IN ('DEPOSIT', 'WITHDRAWAL', 'RESERVE', 'RELEASE')),
	amount     BIGINT      NOT NULL CHECK (amount > 0),
	order_id   UUID,
	message_id UUID,
	result     TEXT        NOT NULL CHECK (result IN ('OK', 'insufficient_funds')),
	created_at TIMESTAMPTZ NOT NULL,
	UNIQUE (type, message_id)
)`

// The id is a UUIDv7, so ordering by id is ordering by creation time.
const createLedgerByUserIndex = `
CREATE INDEX IF NOT EXISTS ledger_by_user ON ledger (user_id, id DESC)`

func createLedger() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			if _, err := d.SQL.Exec(createLedgerTable); err != nil {
				return err
			}

			_, err := d.SQL.Exec(createLedgerByUserIndex)

			return err
		},
	}
}
