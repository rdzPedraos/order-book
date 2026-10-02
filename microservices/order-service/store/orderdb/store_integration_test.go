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

func newPendingOrder(userID string) models.Order {
	price, quantity := int64(9000), int64(10)
	createdAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	return models.Order{
		ID: uuid.Must(uuid.NewV7()), UserID: userID, Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit,
		Limit: &price, Quantity: &quantity, Status: models.StatusPending, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func getStatus(c *require.Assertions, ctx *gofr.Context, order models.Order) models.Status {
	stored, err := GetOrder(ctx, order.UserID, order.ID)
	c.NoError(err)

	return stored.Status
}

// The projector reads the commands and the events without an order between
// them, so the NewOrder and the order's first event can arrive either way.
func TestFirstEventIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("the command before the event", func(t *testing.T) {
		c := require.New(t)
		pending := newPendingOrder("integration-" + uuid.NewString())
		accepted := pending
		accepted.Status = models.StatusOpen

		c.NoError(InsertOrder(ctx, pending))
		c.NoError(InsertOrUpdateOrder(ctx, accepted))
		c.Equal(models.StatusOpen, getStatus(c, ctx, pending))
	})

	t.Run("the event before the command", func(t *testing.T) {
		c := require.New(t)
		pending := newPendingOrder("integration-" + uuid.NewString())
		accepted := pending
		accepted.Status = models.StatusOpen

		c.NoError(InsertOrUpdateOrder(ctx, accepted))
		c.NoError(InsertOrder(ctx, pending))
		c.Equal(models.StatusOpen, getStatus(c, ctx, pending))
	})

	t.Run("a repeated first event leaves a later status", func(t *testing.T) {
		c := require.New(t)
		pending := newPendingOrder("integration-" + uuid.NewString())
		accepted := pending
		accepted.Status = models.StatusOpen
		c.NoError(InsertOrUpdateOrder(ctx, accepted))
		c.NoError(UpdateCancelledOrder(ctx, pending.ID, nil, time.Now()))

		c.NoError(InsertOrUpdateOrder(ctx, accepted))
		c.Equal(models.StatusCancelled, getStatus(c, ctx, pending))
	})
}

func TestEngineUpdatesIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("a modification and a cancellation of an open order", func(t *testing.T) {
		c := require.New(t)
		pending := newPendingOrder("integration-" + uuid.NewString())
		c.NoError(InsertOrder(ctx, pending))

		c.NoError(UpdateModifiedOrder(ctx, pending.ID, 9200, 4, time.Now()))
		reason := "no_liquidity"
		c.NoError(UpdateCancelledOrder(ctx, pending.ID, &reason, time.Now()))
		c.NoError(UpdateModifiedOrder(ctx, pending.ID, 9900, 9, time.Now()))

		stored, err := GetOrder(ctx, pending.UserID, pending.ID)
		c.NoError(err)
		c.Equal([]any{int64(9200), int64(4), models.StatusCancelled, &reason},
			[]any{*stored.Limit, *stored.Quantity, stored.Status, stored.Reason}, "a final order is not modified")
	})
}

func TestInsertTradeIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("a repeated trade is added once and fills both orders", func(t *testing.T) {
		c := require.New(t)
		buy, sell := newPendingOrder("integration-"+uuid.NewString()), newPendingOrder("integration-"+uuid.NewString())
		sell.Side, *sell.Quantity = models.SideSell, 4
		c.NoError(InsertOrder(ctx, buy))
		c.NoError(InsertOrder(ctx, sell))
		trade := models.Trade{ID: uuid.New(), BuyOrderID: buy.ID, SellOrderID: sell.ID, Price: 8500, Quantity: 4, Amount: 34000, CreatedAt: time.Now()}

		c.NoError(InsertTrade(ctx, trade))
		c.NoError(InsertTrade(ctx, trade))

		storedBuy, err := GetOrder(ctx, buy.UserID, buy.ID)
		c.NoError(err)
		c.Equal([]any{models.StatusPartiallyFilled, int64(4), int64(34000)}, []any{storedBuy.Status, storedBuy.FilledQuantity, storedBuy.FilledAmount})
		c.Equal(models.StatusFilled, getStatus(c, ctx, sell))
	})

	t.Run("a market buy is filled when its amount is spent", func(t *testing.T) {
		c := require.New(t)
		buy, sell := newPendingOrder("integration-"+uuid.NewString()), newPendingOrder("integration-"+uuid.NewString())
		amount := int64(30000)
		buy.Type, buy.Limit, buy.Quantity, buy.Amount = models.TypeMarket, nil, nil, &amount
		sell.Side = models.SideSell
		c.NoError(InsertOrder(ctx, buy))
		c.NoError(InsertOrder(ctx, sell))

		c.NoError(InsertTrade(ctx, models.Trade{ID: uuid.New(), BuyOrderID: buy.ID, SellOrderID: sell.ID, Price: 10000, Quantity: 3, Amount: 30000, CreatedAt: time.Now()}))

		c.Equal(models.StatusFilled, getStatus(c, ctx, buy))
	})
}
