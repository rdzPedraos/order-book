package migrations

import "gofr.dev/pkg/gofr/migration"

// The funds role and the trades role write the same balances at the same
// time. If each locked them in the order of its batch, a reservation batch
// holding Ana while it waits for Beto and a trade batch holding Beto while it
// waits for Ana would wait for each other forever, until PostgreSQL cancels
// one. So both functions first lock every balance of their batch in one
// statement, ordered by person and currency: the one that comes second waits
// holding nothing, and no circle can form. Their rules are those of
// 20261006000000 and 20261007000000.
var lockBalancesInOrderStatements = []string{`
CREATE OR REPLACE FUNCTION apply_funds_batch(operations jsonb)
RETURNS TABLE (operation_position int, operation_result text)
LANGUAGE plpgsql AS $$
DECLARE
	operation record;
BEGIN
	PERFORM 1 FROM balances
	WHERE (user_id, currency) IN (
		SELECT batch.user_id, batch.currency FROM jsonb_to_recordset(operations) AS batch(user_id text, currency text))
	ORDER BY user_id, currency
	FOR UPDATE;

	FOR operation IN
		SELECT * FROM jsonb_to_recordset(operations) AS batch(
			position int, id uuid, type text, message_id uuid, order_id uuid,
			user_id text, currency text, amount bigint, created_at timestamptz)
	LOOP
		operation_position := operation.position;

		SELECT ledger.result INTO operation_result FROM ledger
		WHERE ledger.type = operation.type AND ledger.message_id = operation.message_id;

		IF FOUND THEN
			RETURN NEXT;
			CONTINUE;
		END IF;

		IF operation.type = 'RESERVE' THEN
			UPDATE balances SET available = available - operation.amount, reserved = reserved + operation.amount
			WHERE balances.user_id = operation.user_id AND balances.currency = operation.currency
				AND available >= operation.amount;
			operation_result := CASE WHEN FOUND THEN 'OK' ELSE 'insufficient_funds' END;
		ELSE
			UPDATE balances SET reserved = reserved - operation.amount, available = available + operation.amount
			WHERE balances.user_id = operation.user_id AND balances.currency = operation.currency
				AND reserved >= operation.amount;
			operation_result := CASE WHEN FOUND THEN 'OK' ELSE 'release_exceeds_reservation' END;
		END IF;

		IF operation_result <> 'release_exceeds_reservation' THEN
			INSERT INTO ledger (id, user_id, currency, type, amount, order_id, message_id, result, created_at)
			VALUES (operation.id, operation.user_id, operation.currency, operation.type, operation.amount,
				operation.order_id, operation.message_id, operation_result, operation.created_at);
		END IF;

		RETURN NEXT;
	END LOOP;
END
$$`, `
CREATE OR REPLACE FUNCTION apply_trade_movements(movements jsonb)
RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
	movement ledger;
BEGIN
	INSERT INTO balances (user_id, currency)
	SELECT DISTINCT batch.user_id, batch.currency FROM jsonb_populate_recordset(NULL::ledger, movements) AS batch
	ORDER BY batch.user_id, batch.currency
	ON CONFLICT (user_id, currency) DO NOTHING;

	PERFORM 1 FROM balances
	WHERE (user_id, currency) IN (
		SELECT batch.user_id, batch.currency FROM jsonb_populate_recordset(NULL::ledger, movements) AS batch)
	ORDER BY user_id, currency
	FOR UPDATE;

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
			UPDATE balances SET available = available + movement.amount
			WHERE user_id = movement.user_id AND currency = movement.currency;
		END IF;
	END LOOP;
END
$$`,
}

func lockBalancesInOrder() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			for _, statement := range lockBalancesInOrderStatements {
				if _, err := d.SQL.Exec(statement); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
