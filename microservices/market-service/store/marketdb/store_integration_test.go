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

func TestLevelsIntegration(t *testing.T) {
	ctx := newIntegrationContext(t)

	t.Run("levels are written, changed, emptied and read best first", func(t *testing.T) {
		c := require.New(t)
		book := "TEST-" + uuid.NewString()[:8]
		bid := func(price, volume int64, orders int) models.Level {
			return models.Level{Book: book, Side: "BUY", Price: price, Volume: volume, Orders: orders}
		}

		c.NoError(UpdateLevel(ctx, bid(9900, 15, 1)))
		c.NoError(UpdateLevel(ctx, bid(10000, 10, 1)))
		c.NoError(UpdateLevel(ctx, bid(10000, 30, 2)))
		c.NoError(UpdateLevel(ctx, bid(9800, 1, 1)))
		c.NoError(UpdateLevel(ctx, bid(9800, 0, 0)))

		bids, err := ListLevels(ctx, book, "BUY", 20)
		c.NoError(err)
		c.Equal([]models.Level{bid(10000, 30, 2), bid(9900, 15, 1)}, bids)

		asks, err := ListLevels(ctx, book, "SELL", 20)
		c.NoError(err)
		c.Empty(asks)
	})
}
