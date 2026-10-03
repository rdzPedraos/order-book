package orderdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
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

func storedOrder() models.Order {
	createdAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	limit, quantity := int64(9000), int64(10)

	return models.Order{
		ID: uuid.UUID{15: 1}, UserID: "user-a", Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit,
		Limit: &limit, Quantity: &quantity, Status: models.StatusPending, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func orderRows(orders ...models.Order) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "user_id", "book", "side", "type", "limit_price", "amount", "quantity",
		"filled_quantity", "filled_amount", "status", "reason", "created_at", "updated_at"})

	for _, order := range orders {
		rows.AddRow(order.ID.String(), order.UserID, order.Book, string(order.Side), string(order.Type), *order.Limit, nil,
			*order.Quantity, order.FilledQuantity, order.FilledAmount, string(order.Status), nil, order.CreatedAt, order.UpdatedAt)
	}

	return rows
}

func TestPostgresGetOrder(t *testing.T) {
	t.Run("own order is read", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		order := storedOrder()
		mocks.SQL.ExpectQuery(getOrder).WithArgs(order.ID, "user-a").WillReturnRows(orderRows(order))

		got, err := postgres{}.getOrder(ctx, "user-a", order.ID)
		c.NoError(err)
		c.Equal(order, got)
	})

	t.Run("no row is not found", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(getOrder).WillReturnRows(orderRows())

		_, err := postgres{}.getOrder(ctx, "user-b", uuid.UUID{15: 1})
		c.ErrorIs(err, models.ErrOrderNotFound)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(getOrder).WillReturnError(errors.New("connection refused"))

		_, err := postgres{}.getOrder(ctx, "user-a", uuid.UUID{15: 1})
		c.ErrorContains(err, "get order")
	})
}

func TestPostgresListOrders(t *testing.T) {
	query := ListQuery{UserID: "user-a", Limit: 20}

	t.Run("rows are read in order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		newer, older := storedOrder(), storedOrder()
		newer.ID = uuid.UUID{15: 2}
		mocks.SQL.ExpectQuery(listOrders).WithArgs("user-a", nil, nil, nil, nil, 20).WillReturnRows(orderRows(newer, older))

		got, err := postgres{}.listOrders(ctx, query)
		c.NoError(err)
		c.Equal([]models.Order{newer, older}, got)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listOrders).WillReturnError(errors.New("connection refused"))

		_, err := postgres{}.listOrders(ctx, query)
		c.ErrorContains(err, "list orders")
	})

	t.Run("row that cannot be read", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listOrders).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("not-a-uuid"))

		_, err := postgres{}.listOrders(ctx, query)
		c.ErrorContains(err, "scan order")
	})

	t.Run("connection lost while reading", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectQuery(listOrders).WillReturnRows(orderRows(storedOrder()).RowError(0, errors.New("connection reset")))

		_, err := postgres{}.listOrders(ctx, query)
		c.ErrorContains(err, "list orders")
	})
}
