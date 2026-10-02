package marketdb

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
)

// The SQL itself is tested against PostgreSQL (store_integration_test.go).
func newSQLMockContext(t *testing.T) (*gofr.Context, *container.Mocks) {
	t.Helper()

	mockContainer, mocks := container.NewMockContainer(t)
	mocks.Metrics.EXPECT().RecordHistogram(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	return &gofr.Context{Context: context.Background(), Container: mockContainer}, mocks
}

func TestPostgresUpdateLevel(t *testing.T) {
	t.Run("a level with volume is written", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(upsertLevel).WithArgs("BRL-VIB", "BUY", int64(10000), int64(30), 2).WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.updateLevel(ctx, level("BUY", 10000, 30, 2)))
	})

	t.Run("an emptied level is deleted", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(deleteLevel).WithArgs("BRL-VIB", "BUY", int64(10000)).WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.updateLevel(ctx, level("BUY", 10000, 0, 0)))
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(upsertLevel).WillReturnError(errors.New("connection refused"))

		c.ErrorContains(postgres{}.updateLevel(ctx, level("BUY", 10000, 30, 2)), "update level")
	})
}

func TestPostgresListLevels(t *testing.T) {
	t.Run("the rows are read in order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listBids).WithArgs("BRL-VIB", 2).WillReturnRows(sqlmock.NewRows([]string{"price", "volume", "orders"}).
			AddRow(10000, 30, 2).AddRow(9900, 15, 1))

		bids, err := postgres{}.listLevels(ctx, "BRL-VIB", "BUY", 2)
		c.NoError(err)
		c.Equal([]models.Level{level("BUY", 10000, 30, 2), level("BUY", 9900, 15, 1)}, bids)
	})

	t.Run("asks use their own order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listAsks).WithArgs("BRL-VIB", 20).WillReturnRows(sqlmock.NewRows([]string{"price", "volume", "orders"}))

		asks, err := postgres{}.listLevels(ctx, "BRL-VIB", "SELL", 20)
		c.NoError(err)
		c.Empty(asks)
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listBids).WillReturnError(errors.New("connection refused"))

		_, err := postgres{}.listLevels(ctx, "BRL-VIB", "BUY", 20)
		c.ErrorContains(err, "list levels")
	})
}
