package applycommands

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/money"
)

func startLog(t *testing.T) string {
	t.Helper()
	c := require.New(t)

	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "orders.commands", "orders.events"))
	c.NoError(err)
	t.Cleanup(cluster.Close)

	broker := cluster.ListenAddrs()[0]
	c.NoError(producer.Connect([]string{broker}))
	t.Cleanup(producer.Close)

	return broker
}

func readEvents(c *require.Assertions, broker string, count int) []events.Message {
	client, err := kgo.NewClient(kgo.SeedBrokers(broker),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{"orders.events": {0: kgo.NewOffset().AtStart()}}))
	c.NoError(err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var published []events.Message
	for len(published) < count {
		fetches := client.PollRecords(ctx, count-len(published))
		c.NoError(fetches.Err())

		for _, record := range fetches.Records() {
			var message events.Message
			c.NoError(json.Unmarshal(record.Value, &message))
			published = append(published, message)
		}
	}

	return published
}

func TestPublicationFlow(t *testing.T) {
	t.Run("an accepted order and its cancellation reach orders.events", func(t *testing.T) {
		c := require.New(t)
		orderbook.InitEmpty(t)
		wallet := walletclient.InitMock(t)
		wallet.Deposit("ana", money.BRL, 90000)
		broker := startLog(t)

		order := limitBuy(c, "ana", 10, 9000)
		c.NoError(producer.Publish(context.Background(), order))
		c.NoError(producer.Publish(context.Background(), cancelOrder(c, getOrderID(c, order), "ana")))

		runEngineUntil(t, broker, 2)

		published := readEvents(c, broker, 4)
		c.Equal([]string{
			events.RouteOrderAccepted, events.RouteOrderBookLevelChanged, events.RouteOrderCancelled, events.RouteOrderBookLevelChanged,
		}, []string{published[0].Route, published[1].Route, published[2].Route, published[3].Route})
		c.Equal(walletclient.Balance{Available: 90000}, wallet.GetBalance("ana", money.BRL))
	})
}
