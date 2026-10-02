// Package applylevelchanged handles the OrderBookLevelChanged events of
// orders.events: it writes each level as the engine reported it, so the
// public depth follows the book.
package applylevelchanged

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
	"github.com/rdzpedraos/order-book/microservices/market-service/store/marketdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A store failure is returned so the consumer retries the event; a payload
// that cannot be read is logged and skipped.
func Handle(ctx *gofr.Context, message events.Message) error {
	var changed events.OrderBookLevelChanged
	if err := message.ParsePayload(&changed); err != nil {
		ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

		return nil
	}

	return marketdb.UpdateLevel(ctx, models.Level{
		Book: message.Book, Side: changed.Side, Price: changed.Price, Volume: changed.Volume, Orders: changed.Orders,
	})
}
