package events

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var commandID = uuid.MustParse("01923456-7890-7abc-8def-0000000000c1")

func header(sequence uint64, index int) EventHeader {
	return EventHeader{
		Sequence: sequence, Index: index, CommandID: commandID, CommandOffset: 41,
		OrderID: orderID, UserID: "ana",
	}
}

func assertRoundTrip[P any](c *require.Assertions, route string, payload P) {
	message, err := NewEventMessage(route, "BRL-VIB", commandID, 0, createdAt, payload)
	c.NoError(err)

	decoded := roundTrip(c, message)
	c.Equal(route, decoded.Route)

	var parsed P
	c.NoError(decoded.ParsePayload(&parsed))
	c.Equal(payload, parsed)
}

func TestLifecycleRoutes(t *testing.T) {
	c := require.New(t)

	c.Equal("orders.events.OrderAccepted", RouteOrderAccepted)
	c.Equal("orders.events.OrderRejected", RouteOrderRejected)
	c.Equal("orders.events.OrderCancelled", RouteOrderCancelled)
	c.Equal("orders.events.OrderModified", RouteOrderModified)

	topic, err := GetTopic(RouteOrderAccepted)
	c.NoError(err)
	c.Equal(TopicOrderEvents, topic)
}

func TestLifecycleRoundTrip(t *testing.T) {
	t.Run("order accepted", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteOrderAccepted, OrderAccepted{EventHeader: header(7, 0)})
	})

	t.Run("order rejected", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteOrderRejected, OrderRejected{EventHeader: header(7, 0), Reason: ReasonInsufficientFunds})
	})

	t.Run("an accepted order carries its details", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteOrderAccepted, OrderAccepted{
			EventHeader:  header(7, 0),
			OrderDetails: OrderDetails{Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10))},
		})
	})

	t.Run("a rejected market buy carries its amount", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteOrderRejected, OrderRejected{
			EventHeader: header(7, 0), Reason: ReasonInsufficientFunds,
			OrderDetails: OrderDetails{Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))},
		})
	})

	t.Run("the details travel as decimal strings next to the header", func(t *testing.T) {
		c := require.New(t)

		message, err := NewEventMessage(RouteOrderAccepted, "BRL-VIB", commandID, 0, createdAt, OrderAccepted{
			OrderDetails: OrderDetails{Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10))},
		})
		c.NoError(err)
		c.Contains(string(message.Payload), `"side":"BUY","type":"LIMIT","limit":"9000","quantity":"10"`)
	})

	t.Run("order cancelled", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteOrderCancelled, OrderCancelled{
			EventHeader: header(7, 0), CancelledQuantity: 6, Released: 54000, Reason: ReasonNoLiquidity,
		})
	})

	t.Run("order modified", func(t *testing.T) {
		assertRoundTrip(require.New(t), RouteOrderModified, OrderModified{EventHeader: header(7, 0), Limit: ptr(int64(9200)), Quantity: ptr(int64(4))})
	})
}

func getEventID(c *require.Assertions, command uuid.UUID, index int) uuid.UUID {
	message, err := NewEventMessage(RouteOrderAccepted, "BRL-VIB", command, index, createdAt, OrderAccepted{})
	c.NoError(err)

	return message.ID
}

func TestNewEventMessage(t *testing.T) {
	t.Run("same command and index give the same id", func(t *testing.T) {
		c := require.New(t)

		first := getEventID(c, commandID, 0)
		c.Equal(first, getEventID(c, commandID, 0))
		c.Equal(uuid.Version(5), first.Version())
	})

	t.Run("another index of the same command gives another id", func(t *testing.T) {
		c := require.New(t)

		c.NotEqual(getEventID(c, commandID, 0), getEventID(c, commandID, 1))
	})

	t.Run("another command gives another id", func(t *testing.T) {
		c := require.New(t)
		otherCommand := uuid.MustParse("01923456-7890-7abc-8def-0000000000c2")

		c.NotEqual(getEventID(c, commandID, 0), getEventID(c, otherCommand, 0))
	})

	t.Run("envelope of the event", func(t *testing.T) {
		c := require.New(t)

		message, err := NewEventMessage(RouteOrderRejected, "BRL-VIB", commandID, 0, createdAt, OrderRejected{Reason: ReasonInsufficientFunds})
		c.NoError(err)
		c.Equal("BRL-VIB", message.Book)
		c.Equal(SchemaVersion, message.SchemaVersion)
		c.Equal(createdAt, message.CreatedAt)
	})

	t.Run("payload that cannot be encoded", func(t *testing.T) {
		c := require.New(t)

		_, err := NewEventMessage(RouteOrderAccepted, "BRL-VIB", commandID, 0, createdAt, func() {})
		c.Error(err)
	})
}
