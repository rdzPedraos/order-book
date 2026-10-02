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

func TestPostgresInsertOrder(t *testing.T) {
	t.Run("order columns are sent in order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		order := storedOrder()

		mocks.SQL.ExpectExec(insertOrder).WithArgs(order.ID, order.UserID, order.Book, order.Side, order.Type, order.Limit,
			order.Amount, order.Quantity, order.FilledQuantity, order.FilledAmount, order.Status, order.Reason,
			order.CreatedAt, order.UpdatedAt).
			WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.insertOrder(ctx, order))
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(insertOrder).WillReturnError(errors.New("connection refused"))

		c.ErrorContains(postgres{}.insertOrder(ctx, storedOrder()), "insert order")
	})
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

func TestPostgresEngineEvents(t *testing.T) {
	createdAt := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	t.Run("an accepted or rejected order is inserted or updated", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		order := storedOrder()
		mocks.SQL.ExpectExec(insertOrUpdateOrder).WithArgs(order.ID, order.UserID, order.Book, order.Side, order.Type, order.Limit,
			order.Amount, order.Quantity, order.FilledQuantity, order.FilledAmount, order.Status, order.Reason,
			order.CreatedAt, order.UpdatedAt).WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.insertOrUpdateOrder(ctx, order))
	})

	t.Run("a cancelled order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		id, reason := uuid.New(), "no_liquidity"
		mocks.SQL.ExpectExec(updateCancelledOrder).WithArgs(id, &reason, createdAt).WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.updateCancelledOrder(ctx, id, &reason, createdAt))
	})

	t.Run("a modified order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		id := uuid.New()
		mocks.SQL.ExpectExec(updateModifiedOrder).WithArgs(id, int64(9200), int64(2), createdAt).WillReturnResult(sqlmock.NewResult(0, 1))

		c.NoError(postgres{}.updateModifiedOrder(ctx, id, 9200, 2, createdAt))
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectExec(insertOrUpdateOrder).WillReturnError(errors.New("connection refused"))
		mocks.SQL.ExpectExec(updateCancelledOrder).WillReturnError(errors.New("connection refused"))
		mocks.SQL.ExpectExec(updateModifiedOrder).WillReturnError(errors.New("connection refused"))

		c.ErrorContains(postgres{}.insertOrUpdateOrder(ctx, storedOrder()), "insert or update order")
		c.ErrorContains(postgres{}.updateCancelledOrder(ctx, uuid.New(), nil, createdAt), "update cancelled order")
		c.ErrorContains(postgres{}.updateModifiedOrder(ctx, uuid.New(), 9200, 2, createdAt), "update modified order")
	})
}

func TestPostgresInsertTrade(t *testing.T) {
	trade := models.Trade{ID: uuid.New(), BuyOrderID: uuid.New(), SellOrderID: uuid.New(), Price: 9000, Quantity: 4, Amount: 36000,
		CreatedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}

	t.Run("a new trade is added to both orders", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(insertTrade).WithArgs(trade.ID, trade.BuyOrderID, trade.SellOrderID, trade.Price, trade.Quantity,
			trade.Amount, trade.CreatedAt).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(addTradeToOrder).WithArgs(trade.BuyOrderID, trade.Quantity, trade.Amount, trade.CreatedAt).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(addTradeToOrder).WithArgs(trade.SellOrderID, trade.Quantity, trade.Amount, trade.CreatedAt).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectCommit()

		c.NoError(postgres{}.insertTrade(ctx, trade))
	})

	t.Run("a trade already recorded changes no order", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(insertTrade).WillReturnResult(sqlmock.NewResult(0, 0))
		mocks.SQL.ExpectCommit()

		c.NoError(postgres{}.insertTrade(ctx, trade))
	})

	t.Run("a failed update records nothing", func(t *testing.T) {
		c := require.New(t)
		ctx, mocks := newSQLMockContext(t)
		mocks.SQL.ExpectBegin()
		mocks.SQL.ExpectExec(insertTrade).WillReturnResult(sqlmock.NewResult(0, 1))
		mocks.SQL.ExpectExec(addTradeToOrder).WillReturnError(errors.New("connection reset"))
		mocks.SQL.ExpectRollback()

		c.ErrorContains(postgres{}.insertTrade(ctx, trade), "add trade to order")
	})
}
