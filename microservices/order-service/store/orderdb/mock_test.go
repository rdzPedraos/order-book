package orderdb

import (
	"errors"
	"testing"

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
