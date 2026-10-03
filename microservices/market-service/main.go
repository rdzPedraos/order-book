// Command market-service is the public view of the market: the depth of each
// book, by price and volume, built only from the engine's
// OrderBookLevelChanged events. It serves GET /market/orderbook/{book} and
// reads orders.events in the same process.
package main

import (
	"strings"

	"gofr.dev/pkg/gofr"

	applylevelchanges "github.com/rdzpedraos/order-book/microservices/market-service/handlers/apply-level-changes"
	getorderbook "github.com/rdzpedraos/order-book/microservices/market-service/handlers/get-orderbook"
	"github.com/rdzpedraos/order-book/microservices/market-service/migrations"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
)

// The most events the market applies at once, as the engine does.
const levelBatch = 500

func main() {
	app := gofr.New()

	app.Migrate(migrations.All())

	app.GET("/market/orderbook/{book}", getorderbook.Handle)

	brokers := strings.Split(app.Config.Get("EVENT_LOG_BROKERS"), ",")

	// The hook's context ends with the app, which stops the consumer.
	app.OnStart(func(ctx *gofr.Context) error {
		return consumer.StartGroup(ctx, brokers, app.Config.Get("EVENT_LOG_GROUP"), applylevelchanges.Routes,
			levelBatch, ctx.Logger, applylevelchanges.Handle)
	})

	app.Run()
}
