// Command order-service is the public API for placing and querying orders.
//
// It only records the intent of each order as PENDING; matching it against
// the book is the matching engine's job.
package main

import (
	"gofr.dev/pkg/gofr"

	cancelorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/cancel-order"
	createorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/create-order"
	getorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/get-order"
	listorders "github.com/rdzpedraos/order-book/microservices/order-service/handlers/list-orders"
	modifyorder "github.com/rdzpedraos/order-book/microservices/order-service/handlers/modify-order"
	"github.com/rdzpedraos/order-book/microservices/order-service/migrations"
	"github.com/rdzpedraos/order-book/shared/identity"
)

func main() {
	app := gofr.New()

	app.Migrate(migrations.All())
	app.UseMiddleware(identity.Middleware)

	app.POST("/orders", createorder.Handle)
	app.GET("/orders", listorders.Handle)
	app.GET("/orders/{id}", getorder.Handle)
	app.PATCH("/orders/{id}", modifyorder.Handle)
	app.DELETE("/orders/{id}", cancelorder.Handle)

	app.Run()
}
