package migrations

import "gofr.dev/pkg/gofr/migration"

// Applies a batch of the projector's changes in one call, in batch order, so
// each change sees what the earlier ones left: two trades of an order add up.
// Each kind runs the statement it ran on its own, with its condition in the
// WHERE (see orderdb.ChangeKind). The orders and trades go in the batch as
// their table rows. To change a rule, add a migration with the whole new
// function.
const createApplyOrderChangesFunction = `
CREATE OR REPLACE FUNCTION apply_order_changes(changes jsonb)
RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
	change jsonb;
	changed_order orders;
	executed trades;
BEGIN
	FOR change IN SELECT * FROM jsonb_array_elements(changes) LOOP
		CASE change->>'kind'
		WHEN 'insert' THEN
			changed_order := jsonb_populate_record(NULL::orders, change->'order');
			INSERT INTO orders VALUES (changed_order.*)
			ON CONFLICT (id) DO NOTHING;
		WHEN 'first_event' THEN
			changed_order := jsonb_populate_record(NULL::orders, change->'order');
			INSERT INTO orders VALUES (changed_order.*)
			ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, reason = EXCLUDED.reason, updated_at = EXCLUDED.updated_at
			WHERE orders.status = 'PENDING';
		WHEN 'cancel' THEN
			UPDATE orders SET status = 'CANCELLED', reason = change->>'reason', updated_at = (change->>'at')::timestamptz
			WHERE id = (change->>'order_id')::uuid AND status NOT IN ('FILLED', 'CANCELLED', 'REJECTED');
		WHEN 'modify' THEN
			UPDATE orders SET limit_price = (change->>'limit_price')::bigint,
				quantity = filled_quantity + (change->>'pending_quantity')::bigint, updated_at = (change->>'at')::timestamptz
			WHERE id = (change->>'order_id')::uuid AND status NOT IN ('FILLED', 'CANCELLED', 'REJECTED');
		WHEN 'trade' THEN
			executed := jsonb_populate_record(NULL::trades, change->'trade');
			INSERT INTO trades VALUES (executed.*)
			ON CONFLICT (trade_id) DO NOTHING;

			IF FOUND THEN
				UPDATE orders SET
					filled_quantity = filled_quantity + executed.quantity,
					filled_amount = filled_amount + executed.amount,
					status = CASE
						WHEN type = 'MARKET' AND side = 'BUY' AND filled_amount + executed.amount >= amount THEN 'FILLED'
						WHEN NOT (type = 'MARKET' AND side = 'BUY') AND filled_quantity + executed.quantity >= quantity THEN 'FILLED'
						WHEN type = 'LIMIT' THEN 'PARTIALLY_FILLED'
						ELSE status
					END,
					updated_at = executed.created_at
				WHERE id IN (executed.buy_order_id, executed.sell_order_id)
					AND status NOT IN ('FILLED', 'CANCELLED', 'REJECTED');
			END IF;
		END CASE;
	END LOOP;
END
$$`

func createApplyOrderChanges() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createApplyOrderChangesFunction)

			return err
		},
	}
}
