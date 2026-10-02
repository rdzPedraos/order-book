package applycommands

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/money"
)

// The engine's memory is lost; the wallet and the log keep what they had.
func restartEngine(t *testing.T, log *producer.Mock) {
	t.Helper()
	c := require.New(t)
	orderbook.InitEmpty(t)

	last := log.Messages[len(log.Messages)-1]
	c.NoError(ResumeAfter("BRL-VIB", &consumer.Record{Offset: int64(len(log.Messages) - 1), Message: last}))
}

// Runs the engine until it applied sequence commands, then stops it. The
// engine reports its sequence after each batch through a channel, so the test
// reads the book only after the engine wrote it.
func runEngineUntil(t *testing.T, broker string, sequence uint64) {
	t.Helper()
	c := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	applied := make(chan uint64, 100)
	handle := func(ctx *gofr.Context, batch []consumer.Record) error {
		err := Handle(ctx, batch)
		applied <- orderbook.GetBook("BRL-VIB").GetSequence()

		return err
	}

	engineContext := newContext(t)
	engineContext.Context = ctx
	c.NoError(consumer.StartPartition(engineContext, []string{broker}, "orders.commands", 0, 500, engineContext.Logger, handle))

	timeout := time.After(10 * time.Second)
	for {
		select {
		case reached := <-applied:
			if reached == sequence {
				return
			}
		case <-timeout:
			c.Fail("the engine did not apply every command")

			return
		}
	}
}

func TestRestart(t *testing.T) {
	t.Run("restart without repeated events", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("ana", money.BRL, 90000)
		order := limitBuy(c, "ana", 6, 9000)
		batch := toBatch(order, limitBuy(c, "ana", 4, 9000))
		c.NoError(Handle(newContext(t), batch))
		published := len(log.Messages)

		restartEngine(t, log)
		c.NoError(Handle(newContext(t), batch))

		c.Len(log.Messages, published)
		c.Equal(2, orderbook.GetBook("BRL-VIB").Len())
		c.Equal(walletclient.Balance{Reserved: 90000}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("crash before publishing", func(t *testing.T) {
		c := require.New(t)
		wallet, log := setUp(t)
		wallet.Deposit("eva", money.VIB, 3)
		order := newOrder(c, events.NewOrder{UserID: "eva", Side: "SELL", Type: "MARKET", Quantity: ptr(int64(3))})
		c.NoError(Handle(newContext(t), toBatch(order)))
		accepted := log.Messages[0]

		orderbook.InitEmpty(t)
		log.Messages = log.Messages[:1]
		c.NoError(ResumeAfter("BRL-VIB", &consumer.Record{Offset: 0, Message: accepted}))
		c.NoError(Handle(newContext(t), toBatch(order)))

		c.Equal([]string{events.RouteOrderAccepted, events.RouteOrderCancelled}, getRoutes(log))
		c.Equal(walletclient.Balance{Available: 3}, wallet.GetBalance("eva", money.VIB))
	})

	t.Run("nothing published yet", func(t *testing.T) {
		c := require.New(t)
		_, log := setUp(t)

		c.NoError(ResumeAfter("BRL-VIB", nil))
		c.NoError(Handle(newContext(t), toBatch(limitBuy(c, "ana", 1, 9000))))
		c.Len(log.Messages, 1)
	})

	t.Run("a last event that cannot be read", func(t *testing.T) {
		c := require.New(t)
		orderbook.InitEmpty(t)

		err := ResumeAfter("BRL-VIB", &consumer.Record{Message: events.Message{Payload: []byte(`[`)}})
		c.Error(err)
	})

	t.Run("restart after a crash rebuilds the same book", func(t *testing.T) {
		c := require.New(t)
		orderbook.InitEmpty(t)
		wallet := walletclient.InitMock(t)
		wallet.Deposit("ana", money.BRL, 1_000_000_000)
		broker := startLog(t)

		var lastOrder events.Message
		for i := range 1000 {
			if i%10 == 9 {
				c.NoError(producer.Publish(context.Background(), cancelOrder(c, getOrderID(c, lastOrder), "ana")))

				continue
			}

			lastOrder = limitBuy(c, "ana", 1, int64(9000+i))
			c.NoError(producer.Publish(context.Background(), lastOrder))
		}

		runEngineUntil(t, broker, 1000)
		publishedBefore := len(readEvents(c, broker, 2000))
		before := orderbook.GetBook("BRL-VIB").Len()
		balance := wallet.GetBalance("ana", money.BRL)

		orderbook.InitEmpty(t)
		last, err := consumer.GetLastMessage(context.Background(), []string{broker}, "orders.events", 0)
		c.NoError(err)
		c.NoError(ResumeAfter("BRL-VIB", last))
		runEngineUntil(t, broker, 1000)

		c.Equal(before, orderbook.GetBook("BRL-VIB").Len())
		c.Equal(balance, wallet.GetBalance("ana", money.BRL))
		c.Equal(2000, publishedBefore, "each command publishes its order event and its level change")

		end, err := consumer.GetLastMessage(context.Background(), []string{broker}, "orders.events", 0)
		c.NoError(err)
		c.Equal(int64(1999), end.Offset, "no event was published twice")
	})
}
