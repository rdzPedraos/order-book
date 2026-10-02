package applycommands

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

func cancelOrder(c *require.Assertions, orderID uuid.UUID, userID string) events.Message {
	message, err := events.NewMessage(events.RouteCancelOrder, "BRL-VIB", acceptedAt, events.CancelOrder{OrderID: orderID, UserID: userID})
	c.NoError(err)

	return message
}

func TestCancelOrder(t *testing.T) {
	t.Run("cancel an order resting in the book", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 54000)
		order := limitBuy(c, "ana", 6, 9000)
		c.NoError(Handle(newContext(t), toBatch(order)))

		cancel := cancelOrder(c, getOrderID(c, order), "ana")
		c.NoError(Handle(newContext(t), toBatch(cancel)))

		c.Equal(0, orderbook.GetBook("BRL-VIB").Len())
		c.Equal(walletclient.Balance{Available: 54000}, wallet.GetBalance("ana", money.BRL))

		var cancelled events.OrderCancelled
		c.NoError(log.Messages[1].ParsePayload(&cancelled))
		c.Equal(events.RouteOrderCancelled, log.Messages[1].Route)
		c.Equal(int64(54000), cancelled.Released)
		c.Equal(int64(6), cancelled.CancelledQuantity)
		c.Equal(uint64(2), cancelled.Sequence)
		c.Equal(cancel.ID, cancelled.CommandID)
		c.Empty(cancelled.Reason)
	})

	t.Run("cancel a sell gives back its base", func(t *testing.T) {
		c := require.New(t)
		wallet, _ := setUp(t)
		wallet.Deposit("eva", money.VIB, 5)
		order := newOrder(c, events.NewOrder{UserID: "eva", Side: "SELL", Type: "LIMIT", Limit: ptr(int64(12000)), Quantity: ptr(int64(5))})

		c.NoError(Handle(newContext(t), toBatch(order, cancelOrder(c, getOrderID(c, order), "eva"))))

		c.Equal(walletclient.Balance{Available: 5}, wallet.GetBalance("eva", money.VIB))
	})

	t.Run("repeated cancellation", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 54000)
		order := limitBuy(c, "ana", 6, 9000)
		c.NoError(Handle(newContext(t), toBatch(order)))

		c.NoError(Handle(newContext(t), toBatch(cancelOrder(c, getOrderID(c, order), "ana"), cancelOrder(c, getOrderID(c, order), "ana"))))

		c.Equal([]string{events.RouteOrderAccepted, events.RouteOrderCancelled}, getRoutes(log))
		c.Equal(walletclient.Balance{Available: 54000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("cancel an unknown order", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)

		c.NoError(Handle(newContext(t), toBatch(cancelOrder(c, uuid.New(), "ana"))))

		c.Empty(log.Messages)
	})

	t.Run("cancel an order of another person", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 54000)
		order := limitBuy(c, "ana", 6, 9000)

		c.NoError(Handle(newContext(t), toBatch(order, cancelOrder(c, getOrderID(c, order), "luis"))))

		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))
		c.Equal(1, orderbook.GetBook("BRL-VIB").Len())
		c.Equal(walletclient.Balance{Reserved: 54000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("cancel a rejected order", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)
		order := limitBuy(c, "ana", 6, 9000)

		c.NoError(Handle(newContext(t), toBatch(order, cancelOrder(c, getOrderID(c, order), "ana"))))

		c.Equal([]string{events.RouteOrderRejected}, getRoutes(log))
	})

	t.Run("a cancellation whose payload cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)
		malformed := events.Message{ID: uuid.New(), Route: events.RouteCancelOrder, Book: "BRL-VIB", Payload: []byte(`[`)}

		c.NoError(Handle(newContext(t), toBatch(malformed)))
		c.Empty(log.Messages)
		c.Equal(uint64(0), orderbook.GetBook("BRL-VIB").GetSequence())
	})
}
