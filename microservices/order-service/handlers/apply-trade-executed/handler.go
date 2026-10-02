// Package applytradeexecuted handles the TradeExecuted events of
// orders.events: it records the trade and adds it to its buy and sell orders,
// which become PARTIALLY_FILLED or FILLED.
package applytradeexecuted

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A store failure is returned so the consumer retries the event; a payload
// that cannot be read is logged and skipped.
func Handle(ctx *gofr.Context, message events.Message) error {
	var executed events.TradeExecuted
	if err := message.ParsePayload(&executed); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	return orderdb.InsertTrade(ctx, models.Trade{
		ID: executed.TradeID, BuyOrderID: executed.BuyOrderID, SellOrderID: executed.SellOrderID,
		Price: executed.Price, Quantity: executed.Quantity, Amount: executed.Amount, CreatedAt: message.CreatedAt,
	})
}
