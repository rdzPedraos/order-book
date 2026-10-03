package marketdb

import (
	"context"
	"database/sql/driver"
	"encoding/json"
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

type encodedLevels []models.Level

func (expected encodedLevels) Match(value driver.Value) bool {
	encoded, isString := value.(string)
	if !isString {
		return false
	}

	var rows []levelRow
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil || len(rows) != len(expected) {
		return false
	}

	for position, row := range rows {
		level := expected[position]
		if row != (levelRow{Book: level.Book, Side: level.Side, Price: level.Price, Volume: level.Volume, Orders: level.Orders}) {
			return false
		}
	}

	return true
}

func TestPostgresUpdateLevels(t *testing.T) {
	t.Run("a batch is one query with the last state of each level", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(updateLevels).
			WithArgs(encodedLevels{level("BUY", 10000, 30, 2), level("SELL", 10100, 0, 0)}).
			WillReturnResult(sqlmock.NewResult(0, 2))

		c.NoError(postgres{}.updateLevels(ctx, []models.Level{
			level("BUY", 10000, 10, 1), level("SELL", 10100, 5, 1), level("BUY", 10000, 30, 2), level("SELL", 10100, 0, 0),
		}))
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(updateLevels).WillReturnError(errors.New("connection refused"))

		c.ErrorContains(postgres{}.updateLevels(ctx, []models.Level{level("BUY", 10000, 30, 2)}), "update levels")
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
