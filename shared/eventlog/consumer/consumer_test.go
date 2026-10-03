package consumer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
)

// Every test talks to one in-memory Kafka cluster, and creates its own topics.
var brokers string

func TestMain(m *testing.M) {
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1))
	if err != nil {
		panic(err)
	}

	brokers = cluster.ListenAddrs()[0]
	code := m.Run()
	cluster.Close()
	os.Exit(code)
}

type silentLogger struct{}

func (silentLogger) Errorf(string, ...any) {}

func deleteTopic(topic string) {
	admin, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	if err != nil {
		return
	}
	defer admin.Close()

	_, _ = kadm.NewClient(admin).DeleteTopic(context.Background(), topic)
}

func createTopic(t *testing.T) string {
	t.Helper()
	c := require.New(t)

	admin, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	c.NoError(err)
	defer admin.Close()

	topic := "consumer-test-" + uuid.NewString()
	_, err = kadm.NewClient(admin).CreateTopic(context.Background(), 1, 1, nil, topic)
	c.NoError(err)

	t.Cleanup(func() { deleteTopic(topic) })

	return topic
}

func connectProducer(t *testing.T) {
	t.Helper()
	c := require.New(t)

	c.NoError(producer.Connect([]string{brokers}))
	t.Cleanup(producer.Close)
}

func publish(c *require.Assertions, topic, messageType string) events.Message {
	message, err := events.NewMessage(topic+"."+messageType, "BRL-VIB", time.Now(), map[string]string{})
	c.NoError(err)
	c.NoError(producer.Publish(context.Background(), message))

	return message
}

func publishRaw(c *require.Assertions, topic string, value []byte) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	c.NoError(err)
	defer client.Close()

	c.NoError(client.ProduceSync(context.Background(), &kgo.Record{Topic: topic, Value: value}).FirstErr())
}

func startGroup(c *require.Assertions, ctx context.Context, group string, routes []string,
	handle func(context.Context, []events.Message) error,
) {
	c.NoError(StartGroup(ctx, []string{brokers}, group, routes, 500, silentLogger{}, handle))
}

func sendMessagesTo(batches chan []events.Message) func(context.Context, []events.Message) error {
	return func(_ context.Context, batch []events.Message) error {
		batches <- batch

		return nil
	}
}

// Batches can split the messages in any way, so it gathers count of them.
func collectMessageIDs(ctx context.Context, c *require.Assertions, batches chan []events.Message, count int) []uuid.UUID {
	ids := make([]uuid.UUID, 0, count)

	for len(ids) < count {
		select {
		case batch := <-batches:
			for _, message := range batch {
				ids = append(ids, message.ID)
			}
		case <-ctx.Done():
			c.Fail("not every message arrived", "got %d of %d", len(ids), count)

			return ids
		}
	}

	return ids
}

func requireCommitted(c *require.Assertions, group, topic string, offset int64) {
	admin, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	c.NoError(err)
	defer admin.Close()

	c.Eventually(func() bool {
		offsets, err := kadm.NewClient(admin).FetchOffsets(context.Background(), group)
		if err != nil {
			return false
		}

		committed, ok := offsets.Lookup(topic, 0)

		return ok && committed.At == offset
	}, 10*time.Second, 20*time.Millisecond)
}

func TestStartGroup(t *testing.T) {
	t.Run("a batch brings only the messages of its routes, in log order", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		first := publish(c, topic, "Created")
		publish(c, topic, "Ignored")
		second := publish(c, topic, "Created")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []events.Message, 10)
		startGroup(c, ctx, "consumer-test-"+uuid.NewString(), []string{topic + ".Created"}, sendMessagesTo(batches))

		c.Equal([]uuid.UUID{first.ID, second.ID}, collectMessageIDs(ctx, c, batches, 2))
	})

	t.Run("routes of two topics reach the same handler", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		commandsTopic, eventsTopic := createTopic(t), createTopic(t)
		command := publish(c, commandsTopic, "NewOrder")
		event := publish(c, eventsTopic, "TradeExecuted")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []events.Message, 10)
		startGroup(c, ctx, "consumer-test-"+uuid.NewString(),
			[]string{commandsTopic + ".NewOrder", eventsTopic + ".TradeExecuted"}, sendMessagesTo(batches))

		c.ElementsMatch([]uuid.UUID{command.ID, event.ID}, collectMessageIDs(ctx, c, batches, 2))
	})

	t.Run("an applied batch is committed, so the group does not get it again", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		group := "consumer-test-" + uuid.NewString()
		first := publish(c, topic, "Created")

		firstCtx, stopFirst := context.WithTimeout(context.Background(), 20*time.Second)
		defer stopFirst()

		batches := make(chan []events.Message, 10)
		startGroup(c, firstCtx, group, []string{topic + ".Created"}, sendMessagesTo(batches))
		c.Equal([]uuid.UUID{first.ID}, collectMessageIDs(firstCtx, c, batches, 1))
		requireCommitted(c, group, topic, 1)
		stopFirst()

		second := publish(c, topic, "Created")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		restarted := make(chan []events.Message, 10)
		startGroup(c, ctx, group, []string{topic + ".Created"}, sendMessagesTo(restarted))

		c.Equal([]uuid.UUID{second.ID}, collectMessageIDs(ctx, c, restarted, 1))
	})

	t.Run("a failed batch is retried without being committed", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		message := publish(c, topic, "Created")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []events.Message, 10)
		calls := 0
		startGroup(c, ctx, "consumer-test-"+uuid.NewString(), []string{topic + ".Created"},
			func(_ context.Context, batch []events.Message) error {
				calls++
				batches <- batch

				if calls == 1 {
					return errors.New("database unavailable")
				}

				return nil
			})

		c.Equal([]uuid.UUID{message.ID}, collectMessageIDs(ctx, c, batches, 1))
		c.Equal([]uuid.UUID{message.ID}, collectMessageIDs(ctx, c, batches, 1), "the same batch, again")
	})

	t.Run("a record that is not a message is left out", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		publishRaw(c, topic, []byte("not a message"))
		message := publish(c, topic, "Created")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []events.Message, 10)
		startGroup(c, ctx, "consumer-test-"+uuid.NewString(), []string{topic + ".Created"}, sendMessagesTo(batches))

		c.Equal([]uuid.UUID{message.ID}, collectMessageIDs(ctx, c, batches, 1))
	})

	t.Run("a route without a type is rejected", func(t *testing.T) {
		c := require.New(t)

		err := StartGroup(context.Background(), []string{brokers}, "consumer-test-"+uuid.NewString(), []string{"orders"}, 500,
			silentLogger{}, func(context.Context, []events.Message) error { return nil })
		c.ErrorIs(err, events.ErrInvalidRoute)
	})
}
