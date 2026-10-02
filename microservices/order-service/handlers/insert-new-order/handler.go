// Package insertneworder handles the NewOrder commands of orders.commands: it
// inserts each order in orders, so orders is a projection of the log that can
// be rebuilt from offset 0. main.go subscribes it with consumer.Subscribe, which
// retries Handle when it returns an error.
package insertneworder

import (
	"time"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// LagMetric is registered in main.go.
const LagMetric = "order_projection_lag_seconds"

// A store failure is returned so the consumer retries the command; a payload
// that cannot be parsed is logged and skipped, since retrying it would fail
// forever.
func Handle(ctx *gofr.Context, message events.Message) error {
	order, err := buildOrder(message)
	if err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	if err := orderdb.InsertOrder(ctx, order); err != nil {
		return err
	}

	setProjectionLag(ctx, message)

	return nil
}

func buildOrder(message events.Message) (models.Order, error) {
	var newOrder events.NewOrder
	if err := message.ParsePayload(&newOrder); err != nil {
		return models.Order{}, err
	}

	return models.Order{
		ID:        newOrder.OrderID,
		UserID:    newOrder.UserID,
		Book:      message.Book,
		Side:      models.Side(newOrder.Side),
		Type:      models.OrderType(newOrder.Type),
		Limit:     newOrder.Limit,
		Amount:    newOrder.Amount,
		Quantity:  newOrder.Quantity,
		Status:    models.StatusPending,
		CreatedAt: message.CreatedAt,
		UpdatedAt: message.CreatedAt,
	}, nil
}

func setProjectionLag(ctx *gofr.Context, message events.Message) {
	ctx.Metrics().SetGauge(LagMetric, time.Since(message.CreatedAt).Seconds())
}
