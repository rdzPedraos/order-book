package applycommands

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// Crosses the command's order and publishes its trades. What is left of a
// limit order rests in the book; a market order, or one stopped before it
// could cross, is cancelled. What the order no longer needs goes back to the
// wallet in one release: its price improvement and a cancelled remainder.
func crossOrder(ctx *gofr.Context, current *batch, applied *command, order *models.Order) {
	book, err := books.Normalize(applied.record.Message.Book)
	if err != nil {
		ctx.Logger.Errorf("crossing order %s: %v", order.ID, err)

		return
	}

	result := matchOrder(applied.book, book.Base, order)

	for _, trade := range result.trades {
		emitTrade(ctx, current, applied, order, trade)
		applied.touchLevel(trade.MakerSide, trade.Price, order)
	}

	released := result.improvement + settleRemainder(ctx, current, applied, order, result.stopReason)
	if released == 0 {
		return
	}

	currency := book.Quote
	if order.Side == models.SideSell {
		currency = book.Base
	}

	current.release(applied.record.Message.ID, *order, currency, released)
}

func emitTrade(ctx *gofr.Context, current *batch, applied *command, order *models.Order, trade models.Trade) {
	tradeID := events.NewTradeID(applied.record.Message.ID, applied.fills)
	applied.fills++

	current.emit(ctx, applied, events.RouteTradeExecuted, order.ID, order.UserID, func(header events.EventHeader) any {
		return events.TradeExecuted{
			EventHeader: header, TradeID: tradeID, BuyOrderID: trade.BuyOrderID, SellOrderID: trade.SellOrderID,
			BuyerID: trade.BuyerID, SellerID: trade.SellerID, MakerSide: string(trade.MakerSide),
			Price: trade.Price, Quantity: trade.Quantity, Amount: trade.Amount,
		}
	})
}

// Answers what the order still has reserved and must be released: nothing
// for an order that rests, its reservation for one that is cancelled, and any
// rounding left over for one fully executed.
func settleRemainder(ctx *gofr.Context, current *batch, applied *command, order *models.Order, stopReason string) int64 {
	if stopReason == "" && order.Type == models.TypeLimit && order.Quantity > 0 {
		applied.book.Put(order)
		applied.touchLevel(order.Side, order.Price, order)

		return 0
	}

	if !hasRemainder(order) {
		return order.Reserved
	}

	reason := stopReason
	if reason == "" {
		reason = events.ReasonNoLiquidity
	}

	current.emit(ctx, applied, events.RouteOrderCancelled, order.ID, order.UserID, func(header events.EventHeader) any {
		return events.OrderCancelled{EventHeader: header, CancelledQuantity: order.Quantity, Released: order.Reserved, Reason: reason}
	})

	return order.Reserved
}

// A market buy has no quantity, so what is left of it is its money.
func hasRemainder(order *models.Order) bool {
	if isMarketBuy(order) {
		return order.Reserved > 0
	}

	return order.Quantity > 0
}
