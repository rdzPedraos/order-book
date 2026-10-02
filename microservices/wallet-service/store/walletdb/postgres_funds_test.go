package walletdb

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

func expectNoStoredResult(mocks sqlmockLike, operation models.FundsOperation) {
	mocks.ExpectQuery(findResult).WithArgs(operation.Type, operation.MessageID).WillReturnRows(sqlmock.NewRows([]string{"result"}))
}

type sqlmockLike interface {
	ExpectQuery(expectedSQL string) *sqlmock.ExpectedQuery
}

func TestPostgresApplyFundsBatch(t *testing.T) {
	reserve := fundsOperation(models.MovementReserve, "user-a", 800)
	release := fundsOperation(models.MovementRelease, "user-a", 500)

	t.Run("new reservation moves funds and is recorded", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		expectNoStoredResult(mocks.SQL, reserve)
		mocks.SQL.ExpectExec(reserveFunds).WithArgs("user-a", money.BRL, int64(800)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectCommit()

		results, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.NoError(err)
		c.Equal(models.ResultOK, results[0].Result)
	})

	t.Run("reservation without funds is recorded as rejected", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		expectNoStoredResult(mocks.SQL, reserve)
		mocks.SQL.ExpectExec(reserveFunds).WillReturnResult(sqlmock.NewResult(0, 0))
		mocks.SQL.ExpectExec(insertMovement).WithArgs(sqlmock.AnyArg(), "user-a", money.BRL, models.MovementReserve, int64(800),
			reserve.OrderID, reserve.MessageID, models.ResultInsufficientFunds, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectCommit()

		results, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.NoError(err)
		c.Equal(models.ResultInsufficientFunds, results[0].Result)
	})

	t.Run("repeated message returns the stored result", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectQuery(findResult).WillReturnRows(sqlmock.NewRows([]string{"result"}).AddRow("insufficient_funds"))
		mocks.SQL.ExpectCommit()

		results, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.NoError(err)
		c.Equal(models.ResultInsufficientFunds, results[0].Result)
	})

	t.Run("new release moves funds and is recorded", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		expectNoStoredResult(mocks.SQL, release)
		mocks.SQL.ExpectExec(releaseFunds).WithArgs("user-a", money.BRL, int64(500)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectCommit()

		results, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{release})
		c.NoError(err)
		c.Equal(models.ResultOK, results[0].Result)
	})

	t.Run("release beyond the reserved balance is not recorded", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		expectNoStoredResult(mocks.SQL, release)
		mocks.SQL.ExpectExec(releaseFunds).WillReturnResult(sqlmock.NewResult(0, 0))
		mocks.SQL.ExpectCommit()

		results, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{release})
		c.NoError(err)
		c.Equal(models.ResultReleaseExceedsReservation, results[0].Result)
	})

	t.Run("stored result that cannot be read is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectQuery(findResult).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.ErrorContains(err, "find result")
	})

	t.Run("reservation that cannot be written is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		expectNoStoredResult(mocks.SQL, reserve)
		mocks.SQL.ExpectExec(reserveFunds).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{reserve})
		c.ErrorContains(err, "reserve funds")
	})

	t.Run("release that cannot be written is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		expectNoStoredResult(mocks.SQL, release)
		mocks.SQL.ExpectExec(releaseFunds).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{release})
		c.ErrorContains(err, "release funds")
	})

	t.Run("movement that cannot be written is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		expectNoStoredResult(mocks.SQL, release)
		mocks.SQL.ExpectExec(releaseFunds).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		_, err := postgres{}.applyFundsBatch(ctx, []models.FundsOperation{release})
		c.ErrorContains(err, "insert movement")
	})
}
