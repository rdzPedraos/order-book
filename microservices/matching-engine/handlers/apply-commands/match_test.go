package applycommands

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/money"
)

func depositVIB(wallet *walletclient.Mock, userIDs ...string) {
	for _, userID := range userIDs {
		wallet.Deposit(userID, money.VIB, 10)
	}
}

func limitSell(c *require.Assertions, userID string, quantity, price int64) events.Message {
	return newOrder(c, events.NewOrder{UserID: userID, Side: "SELL", Type: "LIMIT", Limit: &price, Quantity: &quantity})
}

func getTrades(c *require.Assertions, log *producer.Mock) []events.TradeExecuted {
	var trades []events.TradeExecuted

	for _, message := range log.Messages {
		if message.Route != events.RouteTradeExecuted {
			continue
		}

		var trade events.TradeExecuted
		c.NoError(message.ParsePayload(&trade))
		trades = append(trades, trade)
	}

	return trades
}

func TestMatchLimitOrder(t *testing.T) {
	t.Run("walk through the book", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "seller-a", "seller-b", "seller-c")
		wallet.Deposit("ana", money.BRL, 60000)
		sellA, sellB, sellC := limitSell(c, "seller-a", 2, 9500), limitSell(c, "seller-b", 3, 9500), limitSell(c, "seller-c", 5, 9800)
		c.NoError(Handle(newContext(t), toBatch(sellA, sellB, sellC)))

		buy := limitBuy(c, "ana", 6, 10000)
		c.NoError(Handle(newContext(t), toBatch(buy)))

		trades := getTrades(c, log)
		c.Len(trades, 3)
		c.Equal(getOrderID(c, sellA), trades[0].SellOrderID)
		c.Equal([]int64{2, 9500, 19000}, []int64{trades[0].Quantity, trades[0].Price, trades[0].Amount})
		c.Equal(getOrderID(c, sellB), trades[1].SellOrderID)
		c.Equal([]int64{3, 9500}, []int64{trades[1].Quantity, trades[1].Price})
		c.Equal(getOrderID(c, sellC), trades[2].SellOrderID)
		c.Equal([]int64{1, 9800}, []int64{trades[2].Quantity, trades[2].Price})
		c.Equal(getOrderID(c, buy), trades[0].BuyOrderID)
		c.Equal("SELL", trades[0].MakerSide)
		c.Equal("ana", trades[0].BuyerID)
		c.Equal("seller-a", trades[0].SellerID)

		_, resting := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, buy))
		c.False(resting)
		level, _ := orderbook.GetBook("BRL-VIB").GetBestLevel("SELL")
		c.Equal(int64(9800), level.Price)
		c.Equal(int64(4), level.Volume)
	})

	t.Run("the limit stops the walk", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "seller-a", "seller-b")
		wallet.Deposit("ana", money.BRL, 50000)
		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "seller-a", 3, 9500), limitSell(c, "seller-b", 2, 10500))))

		buy := limitBuy(c, "ana", 5, 10000)
		c.NoError(Handle(newContext(t), toBatch(buy)))

		trades := getTrades(c, log)
		c.Len(trades, 1)
		c.Equal([]int64{3, 9500}, []int64{trades[0].Quantity, trades[0].Price})

		resting, ok := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, buy))
		c.True(ok)
		c.Equal(int64(2), resting.Quantity)
		c.Equal(int64(10000), resting.Price)
		level, _ := orderbook.GetBook("BRL-VIB").GetBestLevel("SELL")
		c.Equal(int64(10500), level.Price)
	})

	t.Run("a sell crosses the highest buy first", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 9000)
		wallet.Deposit("luis", money.BRL, 9500)
		depositVIB(wallet, "eva")
		low, high := limitBuy(c, "ana", 1, 9000), limitBuy(c, "luis", 1, 9500)
		c.NoError(Handle(newContext(t), toBatch(low, high)))

		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "eva", 1, 9000))))

		trades := getTrades(c, log)
		c.Len(trades, 1)
		c.Equal(getOrderID(c, high), trades[0].BuyOrderID)
		c.Equal(int64(9500), trades[0].Price)
		c.Equal("BUY", trades[0].MakerSide)
	})
}

func TestPriceImprovement(t *testing.T) {
	t.Run("price improvement", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "luis")
		wallet.Deposit("ana", money.BRL, 90000)
		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "luis", 4, 8500))))

		buy := limitBuy(c, "ana", 10, 9000)
		c.NoError(Handle(newContext(t), toBatch(buy)))

		trades := getTrades(c, log)
		c.Len(trades, 1)
		c.Equal([]int64{4, 8500, 34000}, []int64{trades[0].Quantity, trades[0].Price, trades[0].Amount})
		c.Equal(walletclient.Balance{Available: 2000, Reserved: 88000}, wallet.GetBalance("ana", money.BRL))

		resting, ok := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, buy))
		c.True(ok)
		c.Equal(int64(6), resting.Quantity)
		c.Equal(int64(54000), resting.Reserved)
	})

	t.Run("the maker that buys pays its own price and frees nothing", func(t *testing.T) {
		c := require.New(t)
		wallet, _ := setUp(t)
		depositVIB(wallet, "luis")
		wallet.Deposit("ana", money.BRL, 90000)
		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 10, 9000))))

		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "luis", 4, 8500))))

		c.Equal(walletclient.Balance{Reserved: 90000}, wallet.GetBalance("ana", money.BRL))
		c.Equal(walletclient.Balance{Available: 6, Reserved: 4}, wallet.GetBalance("luis", money.VIB))
	})
}

func marketBuy(c *require.Assertions, userID string, amount int64) events.Message {
	return newOrder(c, events.NewOrder{UserID: userID, Side: "BUY", Type: "MARKET", Amount: &amount})
}

func marketSell(c *require.Assertions, userID string, quantity int64) events.Message {
	return newOrder(c, events.NewOrder{UserID: userID, Side: "SELL", Type: "MARKET", Quantity: &quantity})
}

func getCancellations(c *require.Assertions, log *producer.Mock) []events.OrderCancelled {
	var cancellations []events.OrderCancelled

	for _, message := range log.Messages {
		if message.Route != events.RouteOrderCancelled {
			continue
		}

		var cancelled events.OrderCancelled
		c.NoError(message.ParsePayload(&cancelled))
		cancellations = append(cancellations, cancelled)
	}

	return cancellations
}

func TestMatchMarketOrder(t *testing.T) {
	t.Run("market buy with money left over", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "luis", "eva")
		wallet.Deposit("ana", money.BRL, 50000)
		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "luis", 3, 10000), limitSell(c, "eva", 5, 11000))))

		c.NoError(Handle(newContext(t), toBatch(marketBuy(c, "ana", 50000))))

		trades := getTrades(c, log)
		c.Len(trades, 2)
		c.Equal([]int64{3, 10000}, []int64{trades[0].Quantity, trades[0].Price})
		c.Equal([]int64{1, 11000}, []int64{trades[1].Quantity, trades[1].Price})

		cancellations := getCancellations(c, log)
		c.Len(cancellations, 1)
		c.Equal(events.ReasonNoLiquidity, cancellations[0].Reason)
		c.Equal(int64(9000), cancellations[0].Released)
		c.Equal(walletclient.Balance{Available: 9000, Reserved: 41000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("market sell fully executed", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 30000)
		wallet.Deposit("luis", money.BRL, 49500)
		depositVIB(wallet, "eva")
		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 3, 10000), limitBuy(c, "luis", 5, 9900))))

		c.NoError(Handle(newContext(t), toBatch(marketSell(c, "eva", 4))))

		trades := getTrades(c, log)
		c.Len(trades, 2)
		c.Equal([]int64{3, 10000}, []int64{trades[0].Quantity, trades[0].Price})
		c.Equal([]int64{1, 9900}, []int64{trades[1].Quantity, trades[1].Price})
		c.Empty(getCancellations(c, log))
		c.Equal(walletclient.Balance{Available: 6, Reserved: 4}, wallet.GetBalance("eva", money.VIB))
	})

	t.Run("market sell without enough buyers", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 20000)
		depositVIB(wallet, "eva")
		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 2, 10000))))

		c.NoError(Handle(newContext(t), toBatch(marketSell(c, "eva", 5))))

		cancellations := getCancellations(c, log)
		c.Len(cancellations, 1)
		c.Equal(int64(3), cancellations[0].CancelledQuantity)
		c.Equal(walletclient.Balance{Available: 8, Reserved: 2}, wallet.GetBalance("eva", money.VIB))
	})
}

func TestSelfTradePrevention(t *testing.T) {
	t.Run("own counterparty", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "pedro")
		wallet.Deposit("pedro", money.BRL, 10000)
		sell := limitSell(c, "pedro", 1, 9500)
		c.NoError(Handle(newContext(t), toBatch(sell)))

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "pedro", 1, 10000))))

		c.Empty(getTrades(c, log))
		cancellations := getCancellations(c, log)
		c.Len(cancellations, 1)
		c.Equal(events.ReasonSelfTradePrevented, cancellations[0].Reason)
		c.Equal(walletclient.Balance{Available: 10000}, wallet.GetBalance("pedro", money.BRL))

		_, resting := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, sell))
		c.True(resting)
	})

	t.Run("trades before the own counterparty are kept", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "luis", "pedro")
		wallet.Deposit("pedro", money.BRL, 30000)
		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "luis", 1, 9400), limitSell(c, "pedro", 1, 9500))))

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "pedro", 3, 10000))))

		trades := getTrades(c, log)
		c.Len(trades, 1)
		c.Equal("luis", trades[0].SellerID)
		cancellations := getCancellations(c, log)
		c.Len(cancellations, 1)
		c.Equal(int64(2), cancellations[0].CancelledQuantity)
		c.Equal(int64(20000), cancellations[0].Released)
		c.Equal(walletclient.Balance{Available: 20600, Reserved: 9400}, wallet.GetBalance("pedro", money.BRL))
	})
}

func TestModifyPriority(t *testing.T) {
	t.Run("a reduction keeps its place", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "seller-a", "seller-b")
		wallet.Deposit("ana", money.BRL, 9500)
		sellA, sellB := limitSell(c, "seller-a", 3, 9500), limitSell(c, "seller-b", 3, 9500)
		c.NoError(Handle(newContext(t), toBatch(sellA, sellB)))
		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, sellA), "seller-a", nil, ptr(int64(2))))))

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9500))))

		trades := getTrades(c, log)
		c.Len(trades, 1)
		c.Equal(getOrderID(c, sellA), trades[0].SellOrderID)
	})

	t.Run("a new price loses its place", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "seller-a", "seller-b")
		wallet.Deposit("ana", money.BRL, 9500)
		sellA, sellB := limitSell(c, "seller-a", 3, 9600), limitSell(c, "seller-b", 3, 9500)
		c.NoError(Handle(newContext(t), toBatch(sellA, sellB)))
		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, sellA), "seller-a", ptr(int64(9500)), nil))))

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9500))))

		trades := getTrades(c, log)
		c.Len(trades, 1)
		c.Equal(getOrderID(c, sellB), trades[0].SellOrderID)
	})

	t.Run("a new price that crosses", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "luis")
		wallet.Deposit("ana", money.BRL, 30000)
		buy := limitBuy(c, "ana", 3, 9000)
		c.NoError(Handle(newContext(t), toBatch(buy, limitSell(c, "luis", 2, 9500))))

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, buy), "ana", ptr(int64(10000)), nil))))

		trades := getTrades(c, log)
		c.Len(trades, 1)
		c.Equal([]int64{2, 9500}, []int64{trades[0].Quantity, trades[0].Price})
		resting, ok := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, buy))
		c.True(ok)
		c.Equal(int64(1), resting.Quantity)
		c.Equal(int64(10000), resting.Reserved)
		c.Equal(walletclient.Balance{Available: 1000, Reserved: 29000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("a modification that frees funds and crosses frees both in one release", func(t *testing.T) {
		c := require.New(t)
		wallet, _ := setUp(t)
		depositVIB(wallet, "luis")
		wallet.Deposit("ana", money.BRL, 90000)
		buy := limitBuy(c, "ana", 10, 9000)
		c.NoError(Handle(newContext(t), toBatch(buy, limitSell(c, "luis", 5, 9500))))

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, buy), "ana", ptr(int64(10000)), ptr(int64(5))))))

		c.Equal(walletclient.Balance{Available: 42500, Reserved: 47500}, wallet.GetBalance("ana", money.BRL))
	})
}
