package migrations

import "gofr.dev/pkg/gofr/migration"

// Applies a batch of reservations and releases in one call, so the batch is
// one round trip and holds its balance rows only while it runs. Operations go
// in batch order: each one sees the balance the earlier ones left. A message
// already in the ledger answers its stored result and moves nothing; a
// rejected reservation is stored too, so its repetition is rejected again. A
// release that does not fit moves no money and leaves no movement. The results
// are the values of models.Result. To change a rule, add a migration with the
// whole new function.
const createApplyFundsBatchFunction = `
CREATE OR REPLACE FUNCTION apply_funds_batch(operations jsonb)
RETURNS TABLE (operation_position int, operation_result text)
LANGUAGE plpgsql AS $$
DECLARE
	operation record;
BEGIN
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
$$`

func createApplyFundsBatch() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createApplyFundsBatchFunction)

			return err
		},
	}
}
