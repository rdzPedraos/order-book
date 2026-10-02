package applycommands

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/money"
)

func getLevelChanges(c *require.Assertions, log *producer.Mock) []events.OrderBookLevelChanged {
	var changes []events.OrderBookLevelChanged

	for _, message := range log.Messages {
		if message.Route != events.RouteOrderBookLevelChanged {
			continue
		}

		var changed events.OrderBookLevelChanged
		c.NoError(message.ParsePayload(&changed))
		changes = append(changes, changed)
	}

	return changes
}

func getRoutesAfter(log *producer.Mock, published int) []string {
	var routes []string
	for _, message := range log.Messages[published:] {
		routes = append(routes, message.Route)
	}

	return routes
}

func TestLevelChanges(t *testing.T) {
	t.Run("trade with two orders", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "luis")
		wallet.Deposit("ana", money.BRL, 50000)
		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "luis", 2, 9500))))
		published := len(log.Messages)

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 5, 10000))))

		c.Equal([]string{
			events.RouteOrderAccepted, events.RouteTradeExecuted,
			events.RouteOrderBookLevelChanged, events.RouteOrderBookLevelChanged,
		}, getRoutesAfter(log, published))

		var accepted events.OrderAccepted
		c.NoError(log.Messages[published].ParsePayload(&accepted))
		trades, changes := getTrades(c, log), getLevelChanges(c, log)
		c.Equal(accepted.Sequence, trades[0].Sequence)
		c.Equal(accepted.Sequence, changes[len(changes)-1].Sequence)
	})

	t.Run("emptied level", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		depositVIB(wallet, "luis")
		wallet.Deposit("ana", money.BRL, 50000)
		c.NoError(Handle(newContext(t), toBatch(limitSell(c, "luis", 2, 9500))))

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 5, 10000))))

		changes := getLevelChanges(c, log)
		emptied, rested := changes[len(changes)-2], changes[len(changes)-1]
		c.Equal(events.OrderBookLevelChanged{EventHeader: emptied.EventHeader, Side: "SELL", Price: 9500, Volume: 0, Orders: 0}, emptied)
		c.Equal(events.OrderBookLevelChanged{EventHeader: rested.EventHeader, Side: "BUY", Price: 10000, Volume: 3, Orders: 1}, rested)
	})

	t.Run("an order that rests changes its level", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 18000)

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9000), limitBuy(c, "ana", 1, 9000))))

		changes := getLevelChanges(c, log)
		c.Len(changes, 2)
		c.Equal([]int64{1, 2}, []int64{changes[0].Volume, changes[1].Volume})
		c.Equal(2, changes[1].Orders)
	})

	t.Run("a cancellation changes its level", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 9000)
		order := limitBuy(c, "ana", 1, 9000)
		c.NoError(Handle(newContext(t), toBatch(order)))

		c.NoError(Handle(newContext(t), toBatch(cancelOrder(c, getOrderID(c, order), "ana"))))

		changes := getLevelChanges(c, log)
		c.Equal([]int64{9000, 0, 0}, []int64{changes[1].Price, changes[1].Volume, int64(changes[1].Orders)})
	})

	t.Run("a new price changes the old level and the new one", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 9500)
		order := limitBuy(c, "ana", 1, 9000)
		c.NoError(Handle(newContext(t), toBatch(order)))

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, order), "ana", ptr(int64(9500)), nil))))

		changes := getLevelChanges(c, log)
		c.Len(changes, 3)
		c.Equal([]int64{9000, 0}, []int64{changes[1].Price, changes[1].Volume})
		c.Equal([]int64{9500, 1}, []int64{changes[2].Price, changes[2].Volume})
	})

	t.Run("a rejected order changes no level", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9000), cancelOrder(c, uuid.New(), "ana"))))

		c.Empty(getLevelChanges(c, log))
	})
}
