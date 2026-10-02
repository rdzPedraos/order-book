package orderdb

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
)

func order(id byte, userID string, side models.Side) models.Order {
	return models.Order{ID: uuid.UUID{15: id}, UserID: userID, Book: "BRL-VIB", Side: side, Status: models.StatusPending}
}

func ptr[T any](value T) *T {
	return &value
}

func TestMockInsertOrder(t *testing.T) {
	t.Run("order inserted twice is recorded once", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)

		c.NoError(InsertOrder(&gofr.Context{}, order(1, "user-a", models.SideBuy)))
		c.NoError(InsertOrder(&gofr.Context{}, order(1, "user-a", models.SideBuy)))
		c.Equal([]models.Order{order(1, "user-a", models.SideBuy)}, mock.Orders)
	})

	t.Run("order is recorded", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)

		c.NoError(InsertOrder(&gofr.Context{}, order(1, "user-a", models.SideBuy)))
		c.Equal([]models.Order{order(1, "user-a", models.SideBuy)}, mock.Orders)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(InsertOrder(&gofr.Context{}, order(1, "user-a", models.SideBuy)), mock.Err)
		c.Empty(mock.Orders)
	})
}

func TestMockGetOrder(t *testing.T) {
	t.Run("own order", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Orders = []models.Order{order(1, "user-a", models.SideBuy)}

		got, err := GetOrder(&gofr.Context{}, "user-a", uuid.UUID{15: 1})
		c.NoError(err)
		c.Equal(order(1, "user-a", models.SideBuy), got)
	})

	t.Run("order of another person", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Orders = []models.Order{order(1, "user-a", models.SideBuy)}

		_, err := GetOrder(&gofr.Context{}, "user-b", uuid.UUID{15: 1})
		c.ErrorIs(err, models.ErrOrderNotFound)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		_, err := GetOrder(&gofr.Context{}, "user-a", uuid.UUID{15: 1})
		c.ErrorIs(err, mock.Err)
	})
}

func TestMockListOrders(t *testing.T) {
	t.Run("own orders newest first", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Orders = []models.Order{
			order(1, "user-a", models.SideBuy), order(2, "user-b", models.SideBuy), order(3, "user-a", models.SideSell),
		}

		got, err := ListOrders(&gofr.Context{}, ListQuery{UserID: "user-a", Limit: 10})
		c.NoError(err)
		c.Equal([]models.Order{order(3, "user-a", models.SideSell), order(1, "user-a", models.SideBuy)}, got)
	})

	t.Run("filters, cursor and limit", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Orders = []models.Order{
			order(1, "user-a", models.SideBuy), order(2, "user-a", models.SideBuy),
			order(3, "user-a", models.SideSell), order(4, "user-a", models.SideBuy), order(5, "user-a", models.SideBuy),
		}

		got, err := ListOrders(&gofr.Context{}, ListQuery{
			UserID: "user-a", Status: ptr(models.StatusPending), Side: ptr(models.SideBuy), Book: ptr("BRL-VIB"),
			Cursor: ptr(uuid.UUID{15: 5}), Limit: 2,
		})
		c.NoError(err)
		c.Equal([]models.Order{order(4, "user-a", models.SideBuy), order(2, "user-a", models.SideBuy)}, got)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		_, err := ListOrders(&gofr.Context{}, ListQuery{UserID: "user-a", Limit: 10})
		c.ErrorIs(err, mock.Err)
	})
}

func withStatus(stored models.Order, status models.Status) models.Order {
	stored.Status = status

	return stored
}

func TestMockInsertOrUpdateOrder(t *testing.T) {
	t.Run("an order not stored yet is inserted with its status", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		accepted := withStatus(order(1, "user-a", models.SideBuy), models.StatusOpen)

		c.NoError(InsertOrUpdateOrder(&gofr.Context{}, accepted))
		c.Equal([]models.Order{accepted}, mock.Orders)
	})

	t.Run("a pending order takes the event's status", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Orders = []models.Order{order(1, "user-a", models.SideBuy)}
		rejected := withStatus(order(1, "user-a", models.SideBuy), models.StatusRejected)
		rejected.Reason = ptr("insufficient_funds")

		c.NoError(InsertOrUpdateOrder(&gofr.Context{}, rejected))
		c.Equal([]models.Order{rejected}, mock.Orders)
	})

	t.Run("an order out of pending is left as it is", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		filled := withStatus(order(1, "user-a", models.SideBuy), models.StatusFilled)
		mock.Orders = []models.Order{filled}

		c.NoError(InsertOrUpdateOrder(&gofr.Context{}, withStatus(order(1, "user-a", models.SideBuy), models.StatusOpen)))
		c.Equal([]models.Order{filled}, mock.Orders)
	})
}

func TestMockUpdateCancelledOrder(t *testing.T) {
	t.Run("an order not final is cancelled with its reason", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Orders = []models.Order{withStatus(order(1, "user-a", models.SideBuy), models.StatusOpen)}

		c.NoError(UpdateCancelledOrder(&gofr.Context{}, uuid.UUID{15: 1}, ptr("no_liquidity"), time.Time{}))
		c.Equal(models.StatusCancelled, mock.Orders[0].Status)
		c.Equal(ptr("no_liquidity"), mock.Orders[0].Reason)
	})

	t.Run("a final order is left as it is", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		filled := withStatus(order(1, "user-a", models.SideBuy), models.StatusFilled)
		mock.Orders = []models.Order{filled}

		c.NoError(UpdateCancelledOrder(&gofr.Context{}, uuid.UUID{15: 1}, nil, time.Time{}))
		c.Equal([]models.Order{filled}, mock.Orders)
	})
}

func TestMockUpdateModifiedOrder(t *testing.T) {
	t.Run("the quantity is what was executed plus what is pending", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		partial := withStatus(order(1, "user-a", models.SideBuy), models.StatusPartiallyFilled)
		partial.FilledQuantity = 4
		mock.Orders = []models.Order{partial}

		c.NoError(UpdateModifiedOrder(&gofr.Context{}, uuid.UUID{15: 1}, 9200, 2, time.Time{}))
		c.Equal(ptr(int64(9200)), mock.Orders[0].Limit)
		c.Equal(ptr(int64(6)), mock.Orders[0].Quantity)
	})

	t.Run("a final order is left as it is", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		cancelled := withStatus(order(1, "user-a", models.SideBuy), models.StatusCancelled)
		mock.Orders = []models.Order{cancelled}

		c.NoError(UpdateModifiedOrder(&gofr.Context{}, uuid.UUID{15: 1}, 9200, 2, time.Time{}))
		c.Equal([]models.Order{cancelled}, mock.Orders)
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(InsertOrUpdateOrder(&gofr.Context{}, order(1, "user-a", models.SideBuy)), mock.Err)
		c.ErrorIs(UpdateCancelledOrder(&gofr.Context{}, uuid.UUID{15: 1}, nil, time.Time{}), mock.Err)
		c.ErrorIs(UpdateModifiedOrder(&gofr.Context{}, uuid.UUID{15: 1}, 9200, 2, time.Time{}), mock.Err)
	})
}

func TestMockInsertTrade(t *testing.T) {
	t.Run("a trade adds to both orders once", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		buy, sell := withStatus(order(1, "user-a", models.SideBuy), models.StatusOpen), withStatus(order(2, "user-b", models.SideSell), models.StatusOpen)
		buy.Type, sell.Type, buy.Quantity, sell.Quantity = models.TypeLimit, models.TypeLimit, ptr(int64(10)), ptr(int64(4))
		mock.Orders = []models.Order{buy, sell}
		trade := models.Trade{ID: uuid.New(), BuyOrderID: buy.ID, SellOrderID: sell.ID, Price: 9000, Quantity: 4, Amount: 36000}

		c.NoError(InsertTrade(&gofr.Context{}, trade))
		c.NoError(InsertTrade(&gofr.Context{}, trade))

		c.Equal([]models.Trade{trade}, mock.Trades)
		c.Equal([]any{models.StatusPartiallyFilled, int64(4), int64(36000)}, []any{mock.Orders[0].Status, mock.Orders[0].FilledQuantity, mock.Orders[0].FilledAmount})
		c.Equal(models.StatusFilled, mock.Orders[1].Status)
	})

	t.Run("an unavailable database", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(InsertTrade(&gofr.Context{}, models.Trade{}), mock.Err)
	})
}
