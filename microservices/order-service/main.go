// Command order-service is the public API for placing and querying orders.
//
// It runs in one of two roles, set by ROLE: api publishes each order, change
// and close as a command to the log; projector reads the log into the orders
// table: the commands create the orders and the engine's events update them. They are deployed apart because they scale apart. Matching an order
// against the book is the matching engine's job.
package main

import (
	"strings"

	"gofr.dev/pkg/gofr"

	applyordermessages "github.com/rdzpedraos/order-book/microservices/order-service/handlers/apply-order-messages"
	changeorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/change-order"
	closeorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/close-order"
	createorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/create-order"
	getorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/get-order"
	listorders "github.com/rdzpedraos/order-book/microservices/order-service/handlers/list-orders"
	"github.com/rdzpedraos/order-book/microservices/order-service/migrations"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/identity"
)

func main() {
	app := gofr.New()

	app.Migrate(migrations.All())

	brokers := strings.Split(app.Config.Get("COMMAND_LOG_BROKERS"), ",")

	switch role := app.Config.Get("ROLE"); role {
	case "api":
		serveAPI(app, brokers)
	case "projector":
		runProjector(app, brokers)
	default:
		app.Logger().Fatalf("unknown ROLE %q: use api or projector", role)
	}

	app.Run()
}

func serveAPI(app *gofr.App, brokers []string) {
	if err := producer.Connect(brokers); err != nil {
		app.Logger().Fatalf("command log: %v", err)
	}

	app.UseMiddleware(identity.Middleware)

	app.POST("/orders", createorder.Handle)
	app.GET("/orders", listorders.Handle)
	app.GET("/orders/{id}", getorder.Handle)
	app.POST("/orders/{id}/change", changeorder.Handle)
	app.POST("/orders/{id}/close", closeorder.Handle)
}

// The most messages the projector applies at once, as the engine does.
const projectorBatch = 500

// The hook's context ends with the app, which stops the consumer.
func runProjector(app *gofr.App, brokers []string) {
	app.Metrics().NewGauge(applyordermessages.LagMetric, "seconds between accepting a command and storing it in orders")

	app.OnStart(func(ctx *gofr.Context) error {
		return consumer.StartGroup(ctx, brokers, app.Config.Get("COMMAND_LOG_GROUP"), applyordermessages.Routes,
			projectorBatch, ctx.Logger, applyordermessages.Handle)
	})
}
