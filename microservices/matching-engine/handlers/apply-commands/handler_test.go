package applycommands

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/logging"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/money"
)

var acceptedAt = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func ptr[T any](value T) *T {
	return &value
}

func newContextWith(t *testing.T, ctx context.Context) *gofr.Context {
	t.Helper()

	mockContainer, _ := container.NewMockContainer(t)
	mockContainer.Logger = logging.NewLogger(logging.FATAL)

	return &gofr.Context{Context: ctx, Container: mockContainer}
}

func newContext(t *testing.T) *gofr.Context {
	t.Helper()

	return newContextWith(t, context.Background())
}

// A wallet, a log and an empty book until the test ends.
func setUp(t *testing.T) (*walletclient.Mock, *producer.Mock) {
	t.Helper()
	orderbook.InitEmpty(t)

	return walletclient.InitMock(t), producer.InitMock(t)
}

func newOrder(c *require.Assertions, payload events.NewOrder) events.Message {
	if payload.OrderID == uuid.Nil {
		payload.OrderID = uuid.New()
	}

	message, err := events.NewMessage(events.RouteNewOrder, "BRL-VIB", acceptedAt, payload)
	c.NoError(err)

	return message
}

func limitBuy(c *require.Assertions, userID string, quantity, price int64) events.Message {
	return newOrder(c, events.NewOrder{UserID: userID, Side: "BUY", Type: "LIMIT", Limit: &price, Quantity: &quantity})
}

func getOrderID(c *require.Assertions, message events.Message) uuid.UUID {
	var payload struct {
		OrderID uuid.UUID `json:"orderId"`
	}
	c.NoError(message.ParsePayload(&payload))

	return payload.OrderID
}

func toBatch(messages ...events.Message) []consumer.Record {
	batch := make([]consumer.Record, 0, len(messages))
	for offset, message := range messages {
		batch = append(batch, consumer.Record{Offset: int64(offset), Message: message})
	}

	return batch
}

// The published events without the level changes, which TestLevelChanges checks.
func getOrderEvents(log *producer.Mock) []events.Message {
	var orderEvents []events.Message
	for _, message := range log.Messages {
		if message.Route != events.RouteOrderBookLevelChanged {
			orderEvents = append(orderEvents, message)
		}
	}

	return orderEvents
}

func getRoutes(log *producer.Mock) []string {
	var routes []string
	for _, message := range getOrderEvents(log) {
		routes = append(routes, message.Route)
	}

	return routes
}

func getSequence(c *require.Assertions, orderID uuid.UUID) uint64 {
	order, ok := orderbook.GetBook("BRL-VIB").Get(orderID)
	c.True(ok)

	return order.Sequence
}

func TestHandle(t *testing.T) {
	t.Run("concurrent commands are applied one after another, in log order", func(t *testing.T) {
		c := require.New(t)
		wallet, _ := setUp(t)
		orders := []events.Message{limitBuy(c, "ana", 1, 9000), limitBuy(c, "luis", 1, 9000), limitBuy(c, "eva", 1, 9000), limitBuy(c, "juan", 1, 9000)}
		for _, userID := range []string{"ana", "luis", "eva", "juan"} {
			wallet.Deposit(userID, money.BRL, 9000)
		}

		c.NoError(Handle(newContext(t), toBatch(orders...)))

		c.Equal([]uint64{1, 2, 3, 4}, []uint64{
			getSequence(c, getOrderID(c, orders[0])), getSequence(c, getOrderID(c, orders[1])),
			getSequence(c, getOrderID(c, orders[2])), getSequence(c, getOrderID(c, orders[3])),
		})
	})

	t.Run("sequence without gaps", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 30000)

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9000), limitBuy(c, "ana", 1, 9000))))
		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9000))))

		var sequences []uint64
		for _, message := range getOrderEvents(log) {
			var accepted events.OrderAccepted
			c.NoError(message.ParsePayload(&accepted))
			sequences = append(sequences, accepted.Sequence)
		}

		c.Equal([]uint64{1, 2, 3}, sequences)
	})

	t.Run("repeated command in the log", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 100000)
		order := limitBuy(c, "ana", 10, 9000)
		sameOrder := newOrder(c, events.NewOrder{OrderID: getOrderID(c, order), UserID: "ana", Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10))})

		c.NoError(Handle(newContext(t), toBatch(order, order)))
		c.NoError(Handle(newContext(t), toBatch(order, sameOrder)))

		c.Equal(1, orderbook.GetBook("BRL-VIB").Len())
		c.Equal(uint64(1), orderbook.GetBook("BRL-VIB").GetSequence())
		c.Equal(walletclient.Balance{Available: 10000, Reserved: 90000}, wallet.GetBalance("ana", money.BRL))
		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))
	})

	t.Run("insufficient funds", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 50000)
		order := limitBuy(c, "ana", 10, 9000)

		c.NoError(Handle(newContext(t), toBatch(order)))

		c.Equal(0, orderbook.GetBook("BRL-VIB").Len())
		c.Equal([]string{events.RouteOrderRejected}, getRoutes(log))
		c.Equal(walletclient.Balance{Available: 50000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("rejection with its reason", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)
		order := limitBuy(c, "ana", 10, 9000)

		c.NoError(Handle(newContext(t), toBatch(order)))

		var rejected events.OrderRejected
		c.NoError(getOrderEvents(log)[0].ParsePayload(&rejected))
		c.Equal(getOrderID(c, order), rejected.OrderID)
		c.Equal("ana", rejected.UserID)
		c.Equal(events.ReasonInsufficientFunds, rejected.Reason)
	})

	t.Run("enough funds", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 100000)
		order := limitBuy(c, "ana", 10, 9000)

		c.NoError(Handle(newContext(t), toBatch(order)))

		c.Equal(walletclient.Balance{Available: 10000, Reserved: 90000}, wallet.GetBalance("ana", money.BRL))
		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))

		var accepted events.OrderAccepted
		c.NoError(getOrderEvents(log)[0].ParsePayload(&accepted))
		c.Equal(events.EventHeader{Sequence: 1, Index: 0, CommandID: order.ID, CommandOffset: 0, OrderID: getOrderID(c, order), UserID: "ana"}, accepted.EventHeader)
		c.Equal(acceptedAt, getOrderEvents(log)[0].CreatedAt)
	})

	t.Run("limit without counterparty rests in the book", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("eva", money.VIB, 5)
		order := newOrder(c, events.NewOrder{UserID: "eva", Side: "SELL", Type: "LIMIT", Limit: ptr(int64(12000)), Quantity: ptr(int64(5))})

		c.NoError(Handle(newContext(t), toBatch(order)))

		resting, ok := orderbook.GetBook("BRL-VIB").Get(getOrderID(c, order))
		c.True(ok)
		c.Equal(models.Order{
			ID: getOrderID(c, order), UserID: "eva", Side: models.SideSell, Type: models.TypeLimit,
			Price: 12000, Quantity: 5, Reserved: 5, Sequence: 1,
		}, *resting)
		c.Equal(walletclient.Balance{Reserved: 5}, wallet.GetBalance("eva", money.VIB))
		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))
	})

	t.Run("market without counterparty", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("eva", money.VIB, 3)
		order := newOrder(c, events.NewOrder{UserID: "eva", Side: "SELL", Type: "MARKET", Quantity: ptr(int64(3))})

		c.NoError(Handle(newContext(t), toBatch(order)))

		c.Equal(0, orderbook.GetBook("BRL-VIB").Len())
		c.Equal(walletclient.Balance{Available: 3}, wallet.GetBalance("eva", money.VIB))
		c.Equal([]string{events.RouteOrderAccepted, events.RouteOrderCancelled}, getRoutes(log))

		var cancelled events.OrderCancelled
		c.NoError(getOrderEvents(log)[1].ParsePayload(&cancelled))
		c.Equal(events.ReasonNoLiquidity, cancelled.Reason)
		c.Equal(int64(3), cancelled.CancelledQuantity)
		c.Equal(int64(3), cancelled.Released)
		c.Equal(1, cancelled.Index)
	})

	t.Run("market buy reserves its amount", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 50000)
		order := newOrder(c, events.NewOrder{UserID: "ana", Side: "BUY", Type: "MARKET", Amount: ptr(int64(50000))})

		c.NoError(Handle(newContext(t), toBatch(order)))

		c.Equal(walletclient.Balance{Available: 50000}, wallet.GetBalance("ana", money.BRL))
		c.Equal([]string{events.RouteOrderAccepted, events.RouteOrderCancelled}, getRoutes(log))
	})

	t.Run("an order with nothing to reserve is rejected", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		withoutQuantity := newOrder(c, events.NewOrder{UserID: "ana", Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000))})
		tooLarge := newOrder(c, events.NewOrder{UserID: "ana", Side: "BUY", Type: "LIMIT", Limit: ptr(int64(math.MaxInt64)), Quantity: ptr(int64(2))})

		c.NoError(Handle(newContext(t), toBatch(withoutQuantity, tooLarge)))

		c.Equal([]string{events.RouteOrderRejected, events.RouteOrderRejected}, getRoutes(log))
		var rejected events.OrderRejected
		c.NoError(getOrderEvents(log)[1].ParsePayload(&rejected))
		c.Equal(events.ReasonInvalidAmount, rejected.Reason)
		c.Equal(0, wallet.Calls)
	})

	t.Run("all reservations of a batch go in one call", func(t *testing.T) {
		c := require.New(t)
		wallet, _ := setUp(t)
		wallet.Deposit("ana", money.BRL, 100000)

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9000), limitBuy(c, "ana", 1, 9000), limitBuy(c, "ana", 1, 9000))))

		c.Equal(1, wallet.Calls)
	})

	t.Run("a new order whose payload cannot be read is skipped", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)
		malformed := events.Message{ID: uuid.New(), Route: events.RouteNewOrder, Book: "BRL-VIB", Payload: []byte(`{"limit":9000}`)}

		c.NoError(Handle(newContext(t), toBatch(malformed)))
		c.Equal(0, orderbook.GetBook("BRL-VIB").Len())
		c.Equal(uint64(0), orderbook.GetBook("BRL-VIB").GetSequence())
		c.Empty(log.Messages)
	})

	t.Run("a log that does not confirm is retried until it does", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 9000)
		log.FailTimes = 2

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9000))))
		c.Equal([]string{events.RouteOrderAccepted}, getRoutes(log))
	})
}

func TestOrderDetailsInFirstEvent(t *testing.T) {
	t.Run("an accepted order with its details", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 90000)

		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 10, 9000))))

		var accepted events.OrderAccepted
		c.NoError(getOrderEvents(log)[0].ParsePayload(&accepted))
		c.Equal(events.OrderDetails{Side: "BUY", Type: "LIMIT", Limit: ptr(int64(9000)), Quantity: ptr(int64(10))}, accepted.OrderDetails)
	})

	t.Run("a rejected order with its details", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)
		amount := int64(50000)

		c.NoError(Handle(newContext(t), toBatch(newOrder(c, events.NewOrder{UserID: "ana", Side: "BUY", Type: "MARKET", Amount: &amount}))))

		var rejected events.OrderRejected
		c.NoError(getOrderEvents(log)[0].ParsePayload(&rejected))
		c.Equal(events.OrderDetails{Side: "BUY", Type: "MARKET", Amount: &amount}, rejected.OrderDetails)
	})
}
