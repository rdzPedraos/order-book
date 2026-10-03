package migrations

import "gofr.dev/pkg/gofr/migration"

// Pays a batch of trades in one call: the four movements of each trade, in
// batch order, as ledger rows. A movement enters the ledger once, by
// UNIQUE (type, message_id, currency), and only a movement that entered
// moves money: what a person pays leaves their reserved balance and what they
// receive enters their available one. The four movements of a trade differ
// in type or currency, so a trade applied before, or twice in the batch, is
// paid once. CHECK (reserved >= 0) fails the batch if a trade ever took more
// than was reserved. To change a rule, add a migration with the whole new
// function.
const createApplyTradeMovementsFunction = `
CREATE OR REPLACE FUNCTION apply_trade_movements(movements jsonb)
RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
	movement ledger;
BEGIN
	FOR movement IN SELECT * FROM jsonb_populate_recordset(NULL::ledger, movements) LOOP
		INSERT INTO ledger VALUES (movement.*)
		ON CONFLICT (type, message_id, currency) DO NOTHING;

		IF NOT FOUND THEN
			CONTINUE;
		END IF;

		IF movement.type = 'TRADE_PAID' THEN
			UPDATE balances SET reserved = reserved - movement.amount
			WHERE user_id = movement.user_id AND currency = movement.currency;
		ELSE
			INSERT INTO balances (user_id, currency, available)
			VALUES (movement.user_id, movement.currency, movement.amount)
			ON CONFLICT (user_id, currency) DO UPDATE SET available = balances.available + EXCLUDED.available;
		END IF;
	END LOOP;
END
$$`

func createApplyTradeMovements() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createApplyTradeMovementsFunction)

			return err
		},
	}
}
