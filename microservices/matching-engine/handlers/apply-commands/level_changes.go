package applycommands

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A price level a command changed, and the command's order that changed it.
type touchedLevel struct {
	side  models.Side
	price int64
	order *models.Order
}

// Kept in the order levels were first touched, so a replay emits the same events.
func (c *command) touchLevel(side models.Side, price int64, order *models.Order) {
	for _, touched := range c.touchedLevels {
		if touched.side == side && touched.price == price {
			return
		}
	}

	c.touchedLevels = append(c.touchedLevels, touchedLevel{side: side, price: price, order: order})
}

// One event per level the command changed, with the level's whole state once
// the command is applied, so a reader only keeps the last one.
func emitLevelChanges(ctx *gofr.Context, current *batch, applied *command) {
	for _, touched := range applied.touchedLevels {
		var volume int64

		var orders int

		if level, ok := applied.book.GetLevel(touched.side, touched.price); ok {
			volume, orders = level.Volume, level.Count
		}

		current.emit(ctx, applied, events.RouteOrderBookLevelChanged, touched.order.ID, touched.order.UserID, func(header events.EventHeader) any {
			return events.OrderBookLevelChanged{EventHeader: header, Side: string(touched.side), Price: touched.price, Volume: volume, Orders: orders}
		})
	}
}
