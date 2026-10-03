//go:build integration

package marketdb

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/config"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/migration"

	"github.com/rdzpedraos/order-book/microservices/market-service/migrations"
	"github.com/rdzpedraos/order-book/microservices/market-service/models"
)

func newIntegrationContext(t *testing.T) *gofr.Context {
	t.Helper()
	c := require.New(t)

	db := container.NewContainer(config.NewMockConfig(map[string]string{
		"DB_DIALECT": "postgres", "DB_HOST": "localhost", "DB_PORT": "5432", "DB_USER": "exchange",
		"DB_PASSWORD": "exchange", "DB_NAME": "market_service", "DB_SSL_MODE": "disable",
	}))
	c.NotNil(db.SQL, "could not connect to PostgreSQL; run docker compose -f deploy/docker-compose.yml up -d")

	migration.Run(migrations.All(), db)

	return &gofr.Context{Context: context.Background(), Container: db}
}

func newBook() string {
	return "TEST-" + uuid.NewString()[:8]
}

func bid(book string, price, volume int64, orders int) models.Level {
	return models.Level{Book: book, Side: "BUY", Price: price, Volume: volume, Orders: orders}
}

func listStoredBids(c *require.Assertions, ctx *gofr.Context, book string) []models.Level {
	bids, err := ListLevels(ctx, book, "BUY", 20)
	c.NoError(err)

	return bids
}

func TestLevelsIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("a new level and one that changes are read best first", func(t *testing.T) {
		c := require.New(t)
		book := newBook()

		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9900, 15, 1), bid(book, 10000, 10, 1)}))
		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 10000, 30, 2)}))

		c.Equal([]models.Level{bid(book, 10000, 30, 2), bid(book, 9900, 15, 1)}, listStoredBids(c, ctx, book))

		asks, err := ListLevels(ctx, book, "SELL", 20)
		c.NoError(err)
		c.Empty(asks)
	})

	t.Run("emptied level", func(t *testing.T) {
		c := require.New(t)
		book := newBook()
		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9800, 1, 1)}))

		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9800, 0, 0)}))

		c.Empty(listStoredBids(c, ctx, book))
	})

	t.Run("a repeated event writes the same level", func(t *testing.T) {
		c := require.New(t)
		book := newBook()

		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9800, 3, 1)}))
		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9800, 3, 1)}))

		c.Equal([]models.Level{bid(book, 9800, 3, 1)}, listStoredBids(c, ctx, book))
	})

	t.Run("a level that changes twice in a batch keeps its last state", func(t *testing.T) {
		c := require.New(t)
		book := newBook()

		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9700, 1, 1), bid(book, 9700, 6, 3), bid(book, 9700, 4, 2)}))

		c.Equal([]models.Level{bid(book, 9700, 4, 2)}, listStoredBids(c, ctx, book))
	})

	t.Run("a level emptied within its batch is deleted", func(t *testing.T) {
		c := require.New(t)
		book := newBook()
		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9600, 2, 1)}))

		c.NoError(UpdateLevels(ctx, []models.Level{bid(book, 9600, 5, 2), bid(book, 9600, 0, 0), bid(book, 9500, 1, 1)}))

		c.Equal([]models.Level{bid(book, 9500, 1, 1)}, listStoredBids(c, ctx, book))
	})
}
