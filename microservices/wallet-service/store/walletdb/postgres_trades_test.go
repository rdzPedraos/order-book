package walletdb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

func newTrade(buyerID, sellerID string, quantity, amount int64) models.Trade {
	return models.Trade{
		MessageID: uuid.New(), BuyOrderID: uuid.New(), SellOrderID: uuid.New(), BuyerID: buyerID, SellerID: sellerID,
		Base: money.VIB, Quote: money.BRL, Quantity: quantity, Amount: amount, CreatedAt: time.Now().UTC(),
	}
}

type encodedMovements []models.Movement

func (expected encodedMovements) Match(value driver.Value) bool {
	encoded, isString := value.(string)
	if !isString {
		return false
	}

	var rows []movementRow
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil || len(rows) != len(expected) {
		return false
	}

	for position, row := range rows {
		if !isRowOfMovement(row, expected[position]) {
			return false
		}
	}

	return true
}

func isRowOfMovement(row movementRow, movement models.Movement) bool {
	return row.ID != uuid.Nil && row.Type == movement.Type && row.UserID == movement.UserID &&
		row.Currency == movement.Currency && row.Amount == movement.Amount && row.MessageID == *movement.MessageID
}

func buildMovements(c *require.Assertions, trades ...models.Trade) encodedMovements {
	var movements encodedMovements

	for _, trade := range trades {
		built, err := trade.BuildMovements()
		c.NoError(err)

		movements = append(movements, built...)
	}

	return movements
}

func TestPostgresApplyTrades(t *testing.T) {
	t.Run("a batch is one query with the four movements of each trade", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		first, second := newTrade("carla", "victor", 2, 19000), newTrade("victor", "ana", 1, 9500)
		mocks.SQL.ExpectExec(applyTradeMovements).WithArgs(buildMovements(c, first, second)).WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.applyTrades(ctx, []models.Trade{first, second}))
	})

	t.Run("database error is returned", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		connectionReset := errors.New("connection reset")
		mocks.SQL.ExpectExec(applyTradeMovements).WillReturnError(connectionReset)

		c.ErrorIs(postgres{}.applyTrades(ctx, []models.Trade{newTrade("carla", "victor", 2, 19000)}), connectionReset)
	})
}

func TestMockApplyTrades(t *testing.T) {
	t.Run("a repeated trade in a batch moves money once", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Balances = []models.Balance{{UserID: "carla", Currency: money.BRL, Reserved: 19000}, {UserID: "victor", Currency: money.VIB, Reserved: 2}}
		trade := newTrade("carla", "victor", 2, 19000)

		c.NoError(ApplyTrades(nil, []models.Trade{trade, trade}))

		c.Len(mock.Movements, 4)
		c.Equal(int64(19000), mock.getOrCreateBalance("victor", money.BRL).Available)
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(ApplyTrades(nil, []models.Trade{newTrade("carla", "victor", 2, 19000)}), mock.Err)
	})
}
