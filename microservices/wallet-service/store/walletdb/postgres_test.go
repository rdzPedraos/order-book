package walletdb

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

// The SQL itself is tested against PostgreSQL (store_integration_test.go).
// These tests expect the same query constants the code sends, so rewriting a
// query does not break them: they check the arguments and how rows are read.
func newSQLMockContext(t *testing.T) (*gofr.Context, *container.Mocks) {
	t.Helper()

	mockContainer, mocks := container.NewMockContainer(t)
	mocks.Metrics.EXPECT().RecordHistogram(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	return &gofr.Context{Context: context.Background(), Container: mockContainer}, mocks
}

func TestPostgresGetBalances(t *testing.T) {
	t.Run("rows are read", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(getBalances).WithArgs("user-a").WillReturnRows(
			sqlmock.NewRows([]string{"user_id", "currency", "available", "reserved"}).
				AddRow("user-a", "BRL", int64(10000), int64(90000)))

		balances, err := postgres{}.getBalances(ctx, "user-a")
		c.NoError(err)
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 10000, Reserved: 90000}}, balances)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(getBalances).WillReturnError(errors.New("connection refused"))

		_, err := postgres{}.getBalances(ctx, "user-a")
		c.ErrorContains(err, "get balances")
	})

	t.Run("row that cannot be read", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(getBalances).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("user-a"))

		_, err := postgres{}.getBalances(ctx, "user-a")
		c.ErrorContains(err, "scan balance")
	})

	t.Run("connection lost while reading", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(getBalances).WillReturnRows(
			sqlmock.NewRows([]string{"user_id", "currency", "available", "reserved"}).
				AddRow("user-a", "BRL", int64(1), int64(0)).RowError(0, errors.New("connection reset")))

		_, err := postgres{}.getBalances(ctx, "user-a")
		c.ErrorContains(err, "get balances")
	})
}

func TestPostgresDeposit(t *testing.T) {
	movement := deposit("user-a", money.BRL, 15000)

	t.Run("balance and movement in one transaction", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(addAvailable).WithArgs("user-a", money.BRL, int64(15000)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WithArgs(movement.ID, "user-a", money.BRL, models.MovementDeposit, int64(15000),
			movement.OrderID, movement.MessageID, models.ResultOK, movement.CreatedAt).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectCommit()

		c.NoError(postgres{}.deposit(ctx, movement))
	})

	t.Run("balance that cannot be written is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(addAvailable).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		c.ErrorContains(postgres{}.deposit(ctx, movement), "add available")
	})

	t.Run("movement that cannot be written is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(addAvailable).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		c.ErrorContains(postgres{}.deposit(ctx, movement), "insert movement")
	})

	t.Run("transaction that cannot start", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin().WillReturnError(errors.New("connection refused"))

		c.ErrorContains(postgres{}.deposit(ctx, movement), "begin")
	})
}

func TestPostgresWithdraw(t *testing.T) {
	movement := withdrawal("user-a", 4000)

	t.Run("balance and movement in one transaction", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(takeAvailable).WithArgs("user-a", money.BRL, int64(4000)).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(insertMovement).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectCommit()

		c.NoError(postgres{}.withdraw(ctx, movement))
	})

	t.Run("not enough available is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(takeAvailable).WillReturnResult(sqlmock.NewResult(0, 0))
		mocks.SQL.ExpectRollback()

		c.ErrorIs(postgres{}.withdraw(ctx, movement), models.ErrInsufficientFunds)
	})

	t.Run("balance that cannot be written is rolled back", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(takeAvailable).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		c.ErrorContains(postgres{}.withdraw(ctx, movement), "take available")
	})
}

func TestPostgresListMovements(t *testing.T) {
	query := MovementQuery{UserID: "user-a", Limit: 20}
	columns := []string{"id", "user_id", "currency", "type", "amount", "order_id", "message_id", "result", "created_at"}

	t.Run("rows are read", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		movement := deposit("user-a", money.BRL, 15000)
		mocks.SQL.ExpectQuery(listMovements).WithArgs("user-a", nil, nil, nil, 20).WillReturnRows(sqlmock.NewRows(columns).
			AddRow(movement.ID.String(), "user-a", "BRL", "DEPOSIT", int64(15000), nil, nil, "OK", movement.CreatedAt))

		movements, err := postgres{}.listMovements(ctx, query)
		c.NoError(err)
		c.Equal([]models.Movement{movement}, movements)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listMovements).WillReturnError(errors.New("connection refused"))

		_, err := postgres{}.listMovements(ctx, query)
		c.ErrorContains(err, "list movements")
	})

	t.Run("row that cannot be read", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listMovements).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("not-a-uuid"))

		_, err := postgres{}.listMovements(ctx, query)
		c.ErrorContains(err, "scan movement")
	})

	t.Run("connection lost while reading", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		movement := deposit("user-a", money.BRL, 1)
		mocks.SQL.ExpectQuery(listMovements).WillReturnRows(sqlmock.NewRows(columns).
			AddRow(movement.ID.String(), "user-a", "BRL", "DEPOSIT", int64(1), nil, nil, "OK", movement.CreatedAt).
			RowError(0, errors.New("connection reset")))

		_, err := postgres{}.listMovements(ctx, query)
		c.ErrorContains(err, "list movements")
	})
}
