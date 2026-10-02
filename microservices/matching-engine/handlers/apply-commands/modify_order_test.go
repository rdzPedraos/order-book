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

func modifyOrder(c *require.Assertions, orderID uuid.UUID, userID string, limit, quantity *int64) events.Message {
	message, err := events.NewMessage(events.RouteModifyOrder, "BRL-VIB", acceptedAt,
		events.ModifyOrder{OrderID: orderID, UserID: userID, Limit: limit, Quantity: quantity})
	c.NoError(err)

	return message
}

func TestModifyOrder(t *testing.T) {
	t.Run("quantity reduction", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 90000)
		order := limitBuy(c, "ana", 10, 9000)
		c.NoError(Handle(newContext(t), toBatch(order)))

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, order), "ana", nil, ptr(int64(4))))))

		c.Equal(walletclient.Balance{Available: 54000, Reserved: 36000}, wallet.GetBalance("ana", money.BRL))
		resting, _ := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, order))
		c.Equal(int64(4), resting.Quantity)
		c.Equal(int64(36000), resting.Reserved)
		c.Equal(uint64(1), resting.Sequence)
		c.Equal(events.RouteOrderModified, log.Messages[1].Route)

		var modified events.OrderModified
		c.NoError(log.Messages[1].ParsePayload(&modified))
		c.Equal(ptr(int64(4)), modified.Quantity)
		c.Equal(ptr(int64(9000)), modified.Limit)
	})

	t.Run("increase without funds", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 45000)
		order := limitBuy(c, "ana", 5, 9000)
		c.NoError(Handle(newContext(t), toBatch(order)))

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, order), "ana", nil, ptr(int64(50))))))

		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))
		resting, _ := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, order))
		c.Equal(int64(5), resting.Quantity)
		c.Equal(walletclient.Balance{Reserved: 45000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("increase with funds reserves the difference and loses priority", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 100000)
		wallet.Deposit("luis", money.BRL, 9000)
		order, other := limitBuy(c, "ana", 5, 9000), limitBuy(c, "luis", 1, 9000)
		c.NoError(Handle(newContext(t), toBatch(order, other)))

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, order), "ana", nil, ptr(int64(10))))))

		c.Equal(walletclient.Balance{Available: 10000, Reserved: 90000}, wallet.GetBalance("ana", money.BRL))
		level, _ := orderbook.GetBook("BRL-VIB").GetBestLevel("BUY")
		var ids []uuid.UUID
		for resting := range level.Orders() {
			ids = append(ids, resting.ID)
		}

		c.Equal([]uuid.UUID{getOrderID(c, other), getOrderID(c, order)}, ids)
		c.Equal(events.RouteOrderModified, log.Messages[2].Route)
	})

	t.Run("new price moves the order to its new level", func(t *testing.T) {
		c := require.New(t)
		wallet, _ := setUp(t)
		wallet.Deposit("eva", money.VIB, 5)
		order := newOrder(c, events.NewOrder{UserID: "eva", Side: "SELL", Type: "LIMIT", Limit: ptr(int64(12000)), Quantity: ptr(int64(5))})
		c.NoError(Handle(newContext(t), toBatch(order)))

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, getOrderID(c, order), "eva", ptr(int64(11000)), nil))))

		level, _ := orderbook.GetBook("BRL-VIB").GetBestLevel("SELL")
		c.Equal(int64(11000), level.Price)
		c.Equal(walletclient.Balance{Reserved: 5}, wallet.GetBalance("eva", money.VIB))
	})

	t.Run("an order created in the same batch", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 100000)
		order := limitBuy(c, "ana", 5, 9000)

		c.NoError(Handle(newContext(t), toBatch(order, modifyOrder(c, getOrderID(c, order), "ana", nil, ptr(int64(10))))))

		c.Equal(walletclient.Balance{Available: 10000, Reserved: 90000}, wallet.GetBalance("ana", money.BRL))
		c.Equal([]string{events.RouteOrderAccepted, events.RouteOrderModified}, getRoutes(log))
	})

	t.Run("modify an order of another person", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 9000)
		order := limitBuy(c, "ana", 1, 9000)

		c.NoError(Handle(newContext(t), toBatch(order, modifyOrder(c, getOrderID(c, order), "luis", nil, ptr(int64(2))))))

		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))
		resting, _ := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, order))
		c.Equal(int64(1), resting.Quantity)
	})

	t.Run("modify an unknown order", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)

		c.NoError(Handle(newContext(t), toBatch(modifyOrder(c, uuid.New(), "ana", nil, ptr(int64(2))))))

		c.Empty(log.Messages)
	})

	t.Run("modify a final order", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)
		order := limitBuy(c, "ana", 1, 9000)

		c.NoError(Handle(newContext(t), toBatch(order, modifyOrder(c, getOrderID(c, order), "ana", nil, ptr(int64(2))))))

		c.Equal([]string{events.RouteOrderRejected}, getRoutes(log))
	})

	t.Run("a modification too large to reserve", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 9000)
		order := limitBuy(c, "ana", 1, 9000)

		c.NoError(Handle(newContext(t), toBatch(order, modifyOrder(c, getOrderID(c, order), "ana", ptr(int64(1<<62)), ptr(int64(4))))))

		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))
		c.Equal(walletclient.Balance{Reserved: 9000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("a modification whose payload cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)
		malformed := events.Message{ID: uuid.New(), Route: events.RouteModifyOrder, Book: "BRL-VIB", Payload: []byte(`[`)}

		c.NoError(Handle(newContext(t), toBatch(malformed)))
		c.Empty(log.Messages)
	})
}
