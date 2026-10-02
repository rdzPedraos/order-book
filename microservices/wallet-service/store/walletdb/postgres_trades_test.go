package walletdb

import (
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

func TestPostgresApplyTrade(t *testing.T) {
	t.Run("each person pays from reserved and receives in available", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		trade := newTrade("carla", "victor", 2, 19000)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectQuery(isTradeApplied).WithArgs(trade.MessageID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mocks.SQL.ExpectExec(takeReserved).WithArgs("carla", money.BRL, int64(19000)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(addAvailable).WithArgs("carla", money.VIB, int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(takeReserved).WithArgs("victor", money.VIB, int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(addAvailable).WithArgs("victor", money.BRL, int64(19000)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectCommit()

		c.NoError(postgres{}.applyTrade(ctx, trade))
	})

	t.Run("a trade already applied moves nothing", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectQuery(isTradeApplied).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mocks.SQL.ExpectCommit()

		c.NoError(postgres{}.applyTrade(ctx, newTrade("carla", "victor", 2, 19000)))
	})

	t.Run("a failed movement applies nothing", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectQuery(isTradeApplied).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mocks.SQL.ExpectExec(takeReserved).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		c.ErrorContains(postgres{}.applyTrade(ctx, newTrade("carla", "victor", 2, 19000)), "apply TRADE_PAID")
	})
}

func TestMockApplyTrade(t *testing.T) {
	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(ApplyTrade(nil, newTrade("carla", "victor", 2, 19000)), mock.Err)
	})
}
