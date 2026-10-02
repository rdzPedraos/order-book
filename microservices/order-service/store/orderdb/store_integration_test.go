//go:build integration

package orderdb

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/config"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/migration"

	"github.com/rdzpedraos/order-book/microservices/order-service/migrations"
	"github.com/rdzpedraos/order-book/microservices/order-service/models"
)

func newIntegrationContext(t *testing.T) *gofr.Context {
	t.Helper()
	c := require.New(t)

	db := container.NewContainer(config.NewMockConfig(map[string]string{
		"DB_DIALECT": "postgres", "DB_HOST": "localhost", "DB_PORT": "5432", "DB_USER": "exchange",
		"DB_PASSWORD": "exchange", "DB_NAME": "order_service", "DB_SSL_MODE": "disable",
	}))
	c.NotNil(db.SQL, "could not connect to PostgreSQL; run docker compose -f deploy/docker-compose.yml up -d")

	migration.Run(migrations.All(), db)

	return &gofr.Context{Context: context.Background(), Container: db}
}

func insertOrders(t *testing.T, ctx *gofr.Context, userID string, side models.Side, n int) []uuid.UUID {
	t.Helper()
	c := require.New(t)

	ids := make([]uuid.UUID, 0, n)
	price, quantity := int64(9000), int64(1)
	createdAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	for range n {
		order := models.Order{
			ID: uuid.Must(uuid.NewV7()), UserID: userID, Book: "BRL-VIB", Side: side, Type: models.TypeLimit,
			Limit: &price, Quantity: &quantity, Status: models.StatusPending,
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}
		c.NoError(InsertOrder(ctx, order))

		ids = append(ids, order.ID)
	}

	return ids
}

func orderIDs(orders []models.Order) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(orders))
	for _, order := range orders {
		ids = append(ids, order.ID)
	}

	return ids
}

func TestListOrdersIntegration(t *testing.T) {
	t.Run("next page with filters", func(t *testing.T) {
		c := require.New(t)
		ctx := newIntegrationContext(t)
		userID := "it-" + uuid.NewString()
		status, side := models.StatusPending, models.SideBuy

		newestBuys := insertOrders(t, ctx, userID, models.SideBuy, 25)
		slices.Reverse(newestBuys)
		insertOrders(t, ctx, userID, models.SideSell, 3)
		insertOrders(t, ctx, "it-"+uuid.NewString(), models.SideBuy, 3)

		query := ListQuery{UserID: userID, Status: &status, Side: &side, Limit: 21}

		first, err := ListOrders(ctx, query)
		c.NoError(err)
		c.Equal(newestBuys[:21], orderIDs(first))

		cursor := first[19].ID
		query.Cursor = &cursor

		second, err := ListOrders(ctx, query)
		c.NoError(err)
		c.Equal(newestBuys[20:], orderIDs(second))
	})
}

func TestGetOrderIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)
	userID := "get-" + uuid.NewString()
	id := insertOrders(t, ctx, userID, models.SideBuy, 1)[0]

	t.Run("own order", func(t *testing.T) {
		c := require.New(t)

		got, err := GetOrder(ctx, userID, id)
		c.NoError(err)
		c.Equal(id, got.ID)
		c.Equal(userID, got.UserID)
	})

	t.Run("order of another person", func(t *testing.T) {
		c := require.New(t)

		_, err := GetOrder(ctx, "someone-else", id)
		c.ErrorIs(err, models.ErrOrderNotFound)
	})
}

func TestInsertOrderTwiceIntegration(t *testing.T) {
	c := require.New(t)
	ctx := newIntegrationContext(t)
	userID := "twice-" + uuid.NewString()
	id := insertOrders(t, ctx, userID, models.SideBuy, 1)[0]

	order, err := GetOrder(ctx, userID, id)
	c.NoError(err)
	c.NoError(InsertOrder(ctx, order))

	orders, err := ListOrders(ctx, ListQuery{UserID: userID, Limit: 10})
	c.NoError(err)
	c.Len(orders, 1)
}
