package orderdb

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
)

// The statements live in apply_order_changes (migration 20261007000000), so a
// batch is one round trip, and one statement applies all its changes or none.
const applyOrderChanges = `SELECT apply_order_changes($1)`

// The keys are the columns of orders and trades, which the function reads with
// jsonb_populate_record.
type changeRow struct {
	Kind            ChangeKind `json:"kind"`
	Order           *orderRow  `json:"order,omitempty"`
	Trade           *tradeRow  `json:"trade,omitempty"`
	OrderID         uuid.UUID  `json:"order_id"`
	Reason          *string    `json:"reason"`
	LimitPrice      int64      `json:"limit_price,string"`
	PendingQuantity int64      `json:"pending_quantity,string"`
	At              time.Time  `json:"at"`
}

type orderRow struct {
	ID             uuid.UUID        `json:"id"`
	UserID         string           `json:"user_id"`
	Book           string           `json:"book"`
	Side           models.Side      `json:"side"`
	Type           models.OrderType `json:"type"`
	LimitPrice     *int64           `json:"limit_price,string"`
	Amount         *int64           `json:"amount,string"`
	Quantity       *int64           `json:"quantity,string"`
	FilledQuantity int64            `json:"filled_quantity,string"`
	FilledAmount   int64            `json:"filled_amount,string"`
	Status         models.Status    `json:"status"`
	Reason         *string          `json:"reason"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type tradeRow struct {
	TradeID     uuid.UUID `json:"trade_id"`
	BuyOrderID  uuid.UUID `json:"buy_order_id"`
	SellOrderID uuid.UUID `json:"sell_order_id"`
	Price       int64     `json:"price,string"`
	Quantity    int64     `json:"quantity,string"`
	Amount      int64     `json:"amount,string"`
	CreatedAt   time.Time `json:"created_at"`
}

func (postgres) applyChanges(ctx *gofr.Context, changes []Change) error {
	encodedChanges, err := formatChanges(changes)
	if err != nil {
		return err
	}

	if _, err := ctx.SQL.ExecContext(ctx, applyOrderChanges, encodedChanges); err != nil {
		return fmt.Errorf("apply order changes: %w", err)
	}

	return nil
}

func formatChanges(changes []Change) (string, error) {
	rows := make([]changeRow, 0, len(changes))
	for _, change := range changes {
		rows = append(rows, buildChangeRow(change))
	}

	encodedChanges, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("encode order changes: %w", err)
	}

	return string(encodedChanges), nil
}

func buildChangeRow(change Change) changeRow {
	row := changeRow{
		Kind: change.Kind, OrderID: change.OrderID, Reason: change.Reason,
		LimitPrice: change.Limit, PendingQuantity: change.PendingQuantity, At: change.At,
	}

	if order := change.Order; order != nil {
		row.Order = &orderRow{
			ID: order.ID, UserID: order.UserID, Book: order.Book, Side: order.Side, Type: order.Type,
			LimitPrice: order.Limit, Amount: order.Amount, Quantity: order.Quantity,
			FilledQuantity: order.FilledQuantity, FilledAmount: order.FilledAmount, Status: order.Status,
			Reason: order.Reason, CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt,
		}
	}

	if trade := change.Trade; trade != nil {
		row.Trade = &tradeRow{
			TradeID: trade.ID, BuyOrderID: trade.BuyOrderID, SellOrderID: trade.SellOrderID,
			Price: trade.Price, Quantity: trade.Quantity, Amount: trade.Amount, CreatedAt: trade.CreatedAt,
		}
	}

	return row
}
