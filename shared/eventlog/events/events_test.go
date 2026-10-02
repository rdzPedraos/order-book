package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	orderID   = uuid.MustParse("01923456-7890-7abc-8def-0123456789ab")
	createdAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
)

func ptr[T any](value T) *T {
	return &value
}

func roundTrip(c *require.Assertions, message Message) Message {
	encodedMessage, err := json.Marshal(message)
	c.NoError(err)

	var decoded Message
	c.NoError(json.Unmarshal(encodedMessage, &decoded))

	return decoded
}

func TestNewMessage(t *testing.T) {
	t.Run("envelope of the message", func(t *testing.T) {
		c := require.New(t)
		localCreatedAt := time.Date(2026, 10, 1, 7, 0, 0, 0, time.FixedZone("COT", -5*60*60))

		message, err := NewMessage(RouteCancelOrder, "BRL-VIB", localCreatedAt, CancelOrder{OrderID: orderID, UserID: "user-a"})
		c.NoError(err)

		decoded := roundTrip(c, message)
		c.Equal(message.ID, decoded.ID)
		c.Equal(RouteCancelOrder, decoded.Route)
		c.Equal("BRL-VIB", decoded.Book)
		c.Equal(SchemaVersion, decoded.SchemaVersion)
		c.Equal(createdAt, decoded.CreatedAt)
		c.Equal(time.UTC, message.CreatedAt.Location())
	})

	t.Run("each message gets its own uuid v7 id", func(t *testing.T) {
		c := require.New(t)

		first, err := NewMessage(RouteCancelOrder, "BRL-VIB", createdAt, CancelOrder{})
		c.NoError(err)

		second, err := NewMessage(RouteCancelOrder, "BRL-VIB", createdAt, CancelOrder{})
		c.NoError(err)

		c.Equal(uuid.Version(7), first.ID.Version())
		c.NotEqual(first.ID, second.ID)
	})

	t.Run("payload that cannot be encoded", func(t *testing.T) {
		c := require.New(t)

		_, err := NewMessage(RouteNewOrder, "BRL-VIB", createdAt, func() {})
		c.Error(err)
	})
}

func TestParsePayload(t *testing.T) {
	t.Run("payload round trip", func(t *testing.T) {
		c := require.New(t)
		payload := ModifyOrder{OrderID: orderID, UserID: "user-a", Limit: ptr(int64(9200))}

		message, err := NewMessage(RouteModifyOrder, "BRL-VIB", createdAt, payload)
		c.NoError(err)

		var parsed ModifyOrder
		c.NoError(roundTrip(c, message).ParsePayload(&parsed))
		c.Equal(payload, parsed)
	})

	t.Run("malformed payload", func(t *testing.T) {
		c := require.New(t)
		malformed := Message{Route: RouteModifyOrder, Payload: json.RawMessage(`{"limit":9200}`)}

		var parsed ModifyOrder
		c.Error(malformed.ParsePayload(&parsed))
	})
}

func TestGetTopic(t *testing.T) {
	t.Run("topic is the route without its last segment", func(t *testing.T) {
		c := require.New(t)

		topic, err := GetTopic(RouteNewOrder)
		c.NoError(err)
		c.Equal("orders.commands", topic)
	})

	t.Run("route without a type", func(t *testing.T) {
		c := require.New(t)

		_, err := GetTopic("orders")
		c.ErrorIs(err, ErrInvalidRoute)
	})

	t.Run("route without a topic", func(t *testing.T) {
		c := require.New(t)

		_, err := GetTopic(".NewOrder")
		c.ErrorIs(err, ErrInvalidRoute)
	})
}
