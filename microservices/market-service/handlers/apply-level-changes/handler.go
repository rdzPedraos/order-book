// Package applylevelchanges handles the batches of OrderBookLevelChanged
// events: it keeps the levels of each book as the engine last reported them.
package applylevelchanges

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
	"github.com/rdzpedraos/order-book/microservices/market-service/store/marketdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

var Routes = []string{events.RouteOrderBookLevelChanged}

func Handle(ctx *gofr.Context, messages []events.Message) error {
	levels := buildLevels(ctx, messages)
	if len(levels) == 0 {
		return nil
	}

	return marketdb.UpdateLevels(ctx, levels)
}

// A payload that cannot be read never will, so it is logged and left out.
func buildLevels(ctx *gofr.Context, messages []events.Message) []models.Level {
	levels := make([]models.Level, 0, len(messages))

	for _, message := range messages {
		var changed events.OrderBookLevelChanged
		if err := message.ParsePayload(&changed); err != nil {
			ctx.Logger.Errorf("skipping message %s: %v", message.ID, err)

			continue
		}

		levels = append(levels, models.Level{
			Book: message.Book, Side: changed.Side, Price: changed.Price, Volume: changed.Volume, Orders: changed.Orders,
		})
	}

	return levels
}
