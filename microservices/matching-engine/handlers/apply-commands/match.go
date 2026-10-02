package applycommands

import (
	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

// What crossing an incoming order left: its trades, the quote a limit buy no
// longer needs because it paid less than its limit, and why its remainder
// must be cancelled, empty when it may rest.
type matchResult struct {
	trades      []models.Trade
	improvement int64
	stopReason  string
}

// Crosses the incoming order (the taker) against the best opposite prices
// and, at each price, against the oldest order first, at the resting order's
// price. It changes both orders and the book, and does no I/O, so the rules
// are tested and measured without the batch.
func matchOrder(book *orderbook.Book, base money.Currency, taker *models.Order) *matchResult {
	result := &matchResult{}

	for {
		level, ok := book.GetBestLevel(getOppositeSide(taker.Side))
		if !ok || !isCrossing(taker, level.Price) {
			return result
		}

		maker := level.GetFirst()
		if maker.UserID == taker.UserID {
			result.stopReason = events.ReasonSelfTradePrevented

			return result
		}

		trade, improvement, err := executeTrade(book, base, taker, maker)
		if err != nil {
			result.stopReason = events.ReasonInvalidAmount

			return result
		}

		if trade == nil {
			return result
		}

		result.trades = append(result.trades, *trade)
		result.improvement += improvement
	}
}

func getOppositeSide(side models.Side) models.Side {
	if side == models.SideBuy {
		return models.SideSell
	}

	return models.SideBuy
}

// A market order takes any price; a limit buy pays at most its limit and a
// limit sell receives at least its limit.
func isCrossing(taker *models.Order, price int64) bool {
	switch {
	case taker.Type == models.TypeMarket:
		return true
	case taker.Side == models.SideBuy:
		return taker.Price >= price
	default:
		return taker.Price <= price
	}
}

// Nil when the taker cannot afford one unit of the base at the maker's price.
// Every amount is computed before anything changes, so an error leaves both
// orders as they were.
func executeTrade(book *orderbook.Book, base money.Currency, taker, maker *models.Order) (*models.Trade, int64, error) {
	quantity, err := getTradeQuantity(base, taker, maker)
	if err != nil || quantity == 0 {
		return nil, 0, err
	}

	amount, err := money.Notional(base, quantity, maker.Price)
	if err != nil {
		return nil, 0, err
	}

	improvement, err := getImprovement(base, taker, quantity, amount)
	if err != nil {
		return nil, 0, err
	}

	trade := buildTrade(taker, maker, quantity, amount)

	takeFunds(taker, quantity, amount+improvement)
	takeFunds(maker, quantity, amount)
	reduceMaker(book, maker, quantity)

	if !isMarketBuy(taker) {
		taker.Quantity -= quantity
	}

	return trade, improvement, nil
}

// A market buy has no quantity: it takes what its remaining amount covers.
func getTradeQuantity(base money.Currency, taker, maker *models.Order) (int64, error) {
	pending := taker.Quantity

	if isMarketBuy(taker) {
		affordable, err := money.QuantityFor(base, taker.Reserved, maker.Price)
		if err != nil {
			return 0, err
		}

		pending = affordable
	}

	return min(pending, maker.Quantity), nil
}

// A limit buy reserved its limit for this quantity but pays the maker's
// price; the difference is no longer needed. Reserved minus paid, both
// already rounded, so what is paid plus what is freed is what was reserved.
func getImprovement(base money.Currency, taker *models.Order, quantity, amount int64) (int64, error) {
	if taker.Side != models.SideBuy || taker.Type != models.TypeLimit {
		return 0, nil
	}

	reserved, err := money.Notional(base, quantity, taker.Price)
	if err != nil {
		return 0, err
	}

	return reserved - amount, nil
}

func buildTrade(taker, maker *models.Order, quantity, amount int64) *models.Trade {
	buy, sell := taker, maker
	if taker.Side == models.SideSell {
		buy, sell = maker, taker
	}

	return &models.Trade{
		BuyOrderID: buy.ID, SellOrderID: sell.ID, BuyerID: buy.UserID, SellerID: sell.UserID,
		MakerSide: maker.Side, Price: maker.Price, Quantity: quantity, Amount: amount,
	}
}

// What the trade uses stays frozen until the wallet settles it: the amount
// for a buy, the quantity for a sell.
func takeFunds(order *models.Order, quantity, amount int64) {
	if order.Side == models.SideSell {
		order.Reserved -= quantity

		return
	}

	order.Reserved -= amount
}

func reduceMaker(book *orderbook.Book, maker *models.Order, quantity int64) {
	if maker.Quantity == quantity {
		book.Remove(maker.ID)
		maker.Quantity = 0

		return
	}

	book.ReduceQuantity(maker.ID, maker.Quantity-quantity)
}

func isMarketBuy(order *models.Order) bool {
	return order.Type == models.TypeMarket && order.Side == models.SideBuy
}
