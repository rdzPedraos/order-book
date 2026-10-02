package events

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTradeRoutes(t *testing.T) {
	c := require.New(t)

	c.Equal("orders.events.TradeExecuted", RouteTradeExecuted)
	c.Equal("orders.events.OrderBookLevelChanged", RouteOrderBookLevelChanged)

	topic, err := GetTopic(RouteTradeExecuted)
	c.NoError(err)
	c.Equal(TopicOrderEvents, topic)
}

func TestTradeRoundTrip(t *testing.T) {
	t.Run("trade executed", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteTradeExecuted, TradeExecuted{
			EventHeader: header(7, 0), TradeID: NewTradeID(commandID, 0),
			BuyOrderID: orderID, SellOrderID: uuid.MustParse("01923456-7890-7abc-8def-0000000000d2"),
			BuyerID: "ana", SellerID: "luis", MakerSide: "SELL", Price: 8500, Quantity: 4, Amount: 34000,
		})
	})

	t.Run("order book level changed", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteOrderBookLevelChanged, OrderBookLevelChanged{
			EventHeader: header(7, 1), Side: "SELL", Price: 8500, Volume: 0, Orders: 0,
		})
	})

	t.Run("amounts travel as decimal strings", func(t *testing.T) {
		c := require.New(t)

		message, err := NewEventMessage(RouteTradeExecuted, "BRL-VIB", commandID, 0, createdAt, TradeExecuted{Price: 8500, Quantity: 4, Amount: 34000})
		c.NoError(err)
		c.Contains(string(message.Payload), `"price":"8500"`)
		c.Contains(string(message.Payload), `"quantity":"4"`)
		c.Contains(string(message.Payload), `"amount":"34000"`)
	})
}

func TestNewTradeID(t *testing.T) {
	t.Run("same command and fill give the same id", func(t *testing.T) {
		c := require.New(t)

		c.Equal(NewTradeID(commandID, 0), NewTradeID(commandID, 0))
	})

	t.Run("another fill gives another id", func(t *testing.T) {
		c := require.New(t)

		c.NotEqual(NewTradeID(commandID, 0), NewTradeID(commandID, 1))
	})

	t.Run("a trade id never equals an event id of the same command", func(t *testing.T) {
		c := require.New(t)

		c.NotEqual(getEventID(c, commandID, 0), NewTradeID(commandID, 0))
	})
}
