//go:build integration

package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
)

const brokers = "localhost:19092"

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
	c.NoError(err, "run docker compose -f deploy/docker-compose.yml up -d")
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

func startConsumer(c *require.Assertions, ctx context.Context, subscriptions ...Subscription[context.Context]) {
	c.NoError(Start(ctx, []string{brokers}, "consumer-test-"+uuid.NewString(), silentLogger{}, subscriptions...))
}

func sendTo(received chan events.Message) func(context.Context, events.Message) error {
	return func(_ context.Context, message events.Message) error {
		received <- message

		return nil
	}
}

func waitFor(ctx context.Context, c *require.Assertions, received chan events.Message) events.Message {
	select {
	case message := <-received:
		return message
	case <-ctx.Done():
		c.Fail("nothing received")

		return events.Message{}
	}
}

func TestStartIntegration(t *testing.T) {
	t.Run("published message reaches its handler quickly", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		message := publish(c, topic, "Created")
		publishedAt := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		received := make(chan events.Message, 1)
		startConsumer(c, ctx, Subscribe(topic+".Created", sendTo(received)))

		c.Equal(message.ID, waitFor(ctx, c, received).ID)
		c.Less(time.Since(publishedAt), 5*time.Second)
	})

	t.Run("a failed message is retried, not skipped", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		message := publish(c, topic, "Created")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		received := make(chan events.Message, 10)
		calls := 0
		startConsumer(c, ctx, Subscribe(topic+".Created", func(_ context.Context, consumed events.Message) error {
			calls++
			received <- consumed

			if calls == 1 {
				return errors.New("database unavailable")
			}

			return nil
		}))

		c.Equal(message.ID, waitFor(ctx, c, received).ID)
		c.Equal(message.ID, waitFor(ctx, c, received).ID)
	})

	t.Run("record that is not a message is skipped", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		publishRaw(c, topic, []byte("not a message"))
		message := publish(c, topic, "Created")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		received := make(chan events.Message, 1)
		startConsumer(c, ctx, Subscribe(topic+".Created", sendTo(received)))

		c.Equal(message.ID, waitFor(ctx, c, received).ID)
	})

	t.Run("a type without subscription is committed and skipped", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		publish(c, topic, "Ignored")
		message := publish(c, topic, "Created")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		received := make(chan events.Message, 10)
		startConsumer(c, ctx, Subscribe(topic+".Created", sendTo(received)))

		c.Equal(message.ID, waitFor(ctx, c, received).ID)
		c.Empty(received)
	})

	t.Run("each route reaches its own handler", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		commandsTopic, eventsTopic := createTopic(t), createTopic(t)
		command := publish(c, commandsTopic, "NewOrder")
		event := publish(c, eventsTopic, "TradeExecuted")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		receivedCommands := make(chan events.Message, 1)
		receivedEvents := make(chan events.Message, 1)
		startConsumer(c, ctx,
			Subscribe(commandsTopic+".NewOrder", sendTo(receivedCommands)),
			Subscribe(eventsTopic+".TradeExecuted", sendTo(receivedEvents)),
		)

		c.Equal(command.ID, waitFor(ctx, c, receivedCommands).ID)
		c.Equal(event.ID, waitFor(ctx, c, receivedEvents).ID)
	})

	t.Run("route without a type is rejected", func(t *testing.T) {
		c := require.New(t)

		err := Start(context.Background(), []string{brokers}, "consumer-test-"+uuid.NewString(), silentLogger{},
			Subscribe("orders", func(context.Context, events.Message) error { return nil }))
		c.ErrorIs(err, events.ErrInvalidRoute)
	})
}
