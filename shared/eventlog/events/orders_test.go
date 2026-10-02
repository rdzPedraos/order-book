package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderRoutes(t *testing.T) {
	c := require.New(t)

	c.Equal("orders.commands.NewOrder", RouteNewOrder)
	c.Equal("orders.commands.ModifyOrder", RouteModifyOrder)
	c.Equal("orders.commands.CancelOrder", RouteCancelOrder)

	topic, err := GetTopic(RouteNewOrder)
	c.NoError(err)
	c.Equal(TopicOrderCommands, topic)
}

func TestNewOrderPayload(t *testing.T) {
	t.Run("amounts travel as decimal strings", func(t *testing.T) {
		c := require.New(t)

		encodedPayload, err := json.Marshal(NewOrder{OrderID: orderID, UserID: "user-a", Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))})
		c.NoError(err)

		c.JSONEq(`{"orderId":"`+orderID.String()+`","userId":"user-a","side":"BUY","type":"MARKET","amount":"50000"}`, string(encodedPayload))
	})

	t.Run("new order round trip", func(t *testing.T) {
		c := require.New(t)
		payload := NewOrder{OrderID: orderID, UserID: "user-a", Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10))}

		message, err := NewMessage(RouteNewOrder, "BRL-VIB", createdAt, payload)
		c.NoError(err)

		var parsed NewOrder
		c.NoError(roundTrip(c, message).ParsePayload(&parsed))
		c.Equal(payload, parsed)
	})
}
