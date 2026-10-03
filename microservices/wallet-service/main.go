// Command wallet-service is the authority over the money of each person: their
// balances in BRL and VIB, deposits, withdrawals, and the reservations the
// matching engine makes before an order enters the book.
//
// It runs in one of three roles, set by ROLE: api serves the public routes of
// the people; funds serves only POST /wallet/internal/funds:batch for the
// matching engine; trades serves no route and pays the trades the engine
// publishes to orders.events. A role never registers another's routes, so the
// internal route does not exist where people arrive, and a backlog of trades
// never competes with the engine's reservations.
package main

import (
	"strings"

	"gofr.dev/pkg/gofr"

	applyfundsbatch "github.com/rdzpedraos/order-book/microservices/wallet-service/handlers/apply-funds-batch"
	applytrades "github.com/rdzpedraos/order-book/microservices/wallet-service/handlers/apply-trades"
	createdeposit "github.com/rdzpedraos/order-book/microservices/wallet-service/handlers/create-deposit"
	createwithdrawal "github.com/rdzpedraos/order-book/microservices/wallet-service/handlers/create-withdrawal"
	getwallet "github.com/rdzpedraos/order-book/microservices/wallet-service/handlers/get-wallet"
	listmovements "github.com/rdzpedraos/order-book/microservices/wallet-service/handlers/list-movements"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/migrations"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/identity"
)

func main() {
	buildApp().Run()
}

func buildApp() *gofr.App {
	app := gofr.New()

	app.Migrate(migrations.All())

	switch role := app.Config.Get("ROLE"); role {
	case "api":
		serveAPI(app)
	case "funds":
		serveFunds(app)
	case "trades":
		runTradeConsumer(app)
	default:
		app.Logger().Fatalf("unknown ROLE %q: use api, funds or trades", role)
	}

	return app
}

func serveAPI(app *gofr.App) {
	app.UseMiddleware(identity.Middleware)

	app.GET("/wallet", getwallet.Handle)
	app.POST("/wallet/deposits", createdeposit.Handle)
	app.POST("/wallet/withdrawals", createwithdrawal.Handle)
	app.GET("/wallet/movements", listmovements.Handle)
}

// Each operation carries its own userId, so there is no X-User-ID here.
func serveFunds(app *gofr.App) {
	app.POST("/wallet/internal/funds:batch", applyfundsbatch.Handle)
}

// The most trades the trades role pays at once, as the engine does.
const tradeBatch = 500

// The hook's context ends with the app, which stops the consumer.
func runTradeConsumer(app *gofr.App) {
	brokers := strings.Split(app.Config.Get("EVENT_LOG_BROKERS"), ",")

	app.OnStart(func(ctx *gofr.Context) error {
		return consumer.StartGroup(ctx, brokers, app.Config.Get("EVENT_LOG_GROUP"), applytrades.Routes,
			tradeBatch, ctx.Logger, applytrades.Handle)
	})
}
