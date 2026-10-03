package orderdb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
)

type encodedChanges struct {
	kinds []ChangeKind
	ids   []uuid.UUID
}

func (expected encodedChanges) Match(value driver.Value) bool {
	encoded, isString := value.(string)
	if !isString {
		return false
	}

	var rows []changeRow
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil || len(rows) != len(expected.kinds) {
		return false
	}

	for position, row := range rows {
		if row.Kind != expected.kinds[position] || getChangedID(row) != expected.ids[position] {
			return false
		}
	}

	return true
}

func getChangedID(row changeRow) uuid.UUID {
	switch {
	case row.Order != nil:
		return row.Order.ID
	case row.Trade != nil:
		return row.Trade.TradeID
	default:
		return row.OrderID
	}
}

func TestPostgresApplyChanges(t *testing.T) {
	pending := order(1, "user-a", models.SideBuy)
	trade := models.Trade{ID: uuid.UUID{15: 9}, BuyOrderID: pending.ID, SellOrderID: uuid.UUID{15: 2}, Price: 9000, Quantity: 1, Amount: 9000}

	t.Run("a batch is one query with its changes in order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(applyOrderChanges).
			WithArgs(encodedChanges{kinds: []ChangeKind{ChangeInsert, ChangeTrade, ChangeCancel}, ids: []uuid.UUID{pending.ID, trade.ID, pending.ID}}).
			WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.applyChanges(ctx, []Change{
			{Kind: ChangeInsert, Order: &pending},
			{Kind: ChangeTrade, Trade: &trade},
			{Kind: ChangeCancel, OrderID: pending.ID, At: time.Now()},
		}))
	})

	t.Run("database error is returned", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		connectionReset := errors.New("connection reset")
		mocks.SQL.ExpectExec(applyOrderChanges).WillReturnError(connectionReset)

		c.ErrorIs(postgres{}.applyChanges(ctx, []Change{{Kind: ChangeInsert, Order: &pending}}), connectionReset)
	})
}
