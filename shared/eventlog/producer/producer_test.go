package producer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
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

	topic := "producer-test-" + uuid.NewString()
	_, err = kadm.NewClient(admin).CreateTopic(context.Background(), 1, 1, nil, topic)
	c.NoError(err)

	t.Cleanup(func() { deleteTopic(topic) })

	return topic
}

func connect(t *testing.T) {
	t.Helper()
	c := require.New(t)

	c.NoError(Connect([]string{brokers}))
	t.Cleanup(Close)
}

func readTopic(c *require.Assertions, topic string, count int) []*kgo.Record {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: {0: kgo.NewOffset().AtStart()}}))
	c.NoError(err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var records []*kgo.Record
	for len(records) < count {
		fetches := client.PollRecords(ctx, count-len(records))
		c.NoError(fetches.Err())
		records = append(records, fetches.Records()...)
	}

	return records
}

func getMessageIDs(c *require.Assertions, records []*kgo.Record) []uuid.UUID {
	messageIDs := make([]uuid.UUID, 0, len(records))

	for _, record := range records {
		var message events.Message
		c.NoError(json.Unmarshal(record.Value, &message))
		messageIDs = append(messageIDs, message.ID)
	}

	return messageIDs
}

func newMessageOn(c *require.Assertions, topic, book string) events.Message {
	message, err := events.NewMessage(topic+".Test", book, time.Now(), map[string]string{})
	c.NoError(err)

	return message
}

func mustGetBook(c *require.Assertions, id string) books.Book {
	book, err := books.Normalize(id)
	c.NoError(err)

	return book
}

func TestPublishIntegration(t *testing.T) {
	t.Run("message lands in the partition of its book, keyed by the book", func(t *testing.T) {
		c := require.New(t)
		connect(t)
		topic := createTopic(t)
		message := newMessageOn(c, topic, "brl-vib")

		c.NoError(Publish(context.Background(), message))

		record := readTopic(c, topic, 1)[0]
		c.Equal(int32(0), record.Partition)
		c.Equal("BRL-VIB", string(record.Key))
		c.Equal([]uuid.UUID{message.ID}, getMessageIDs(c, []*kgo.Record{record}))
		c.GreaterOrEqual(record.ProducerID, int64(0), "the producer is idempotent, so the broker drops retried copies")
	})

	t.Run("order preserved by instance", func(t *testing.T) {
		c := require.New(t)
		connect(t)
		topic := createTopic(t)
		first, second, third := newMessageOn(c, topic, "BRL-VIB"), newMessageOn(c, topic, "BRL-VIB"), newMessageOn(c, topic, "BRL-VIB")

		c.NoError(Publish(context.Background(), first))
		c.NoError(Publish(context.Background(), second))
		c.NoError(Publish(context.Background(), third))

		c.Equal([]uuid.UUID{first.ID, second.ID, third.ID}, getMessageIDs(c, readTopic(c, topic, 3)))
	})

	t.Run("same book, same partition from several instances", func(t *testing.T) {
		c := require.New(t)
		connect(t)
		topic := createTopic(t)

		other, err := connectProducer([]string{brokers})
		c.NoError(err)
		defer other.close()

		first, second := newMessageOn(c, topic, "BRL-VIB"), newMessageOn(c, topic, "BRL-VIB")
		c.NoError(Publish(context.Background(), first))
		c.NoError(other.publish(context.Background(), topic, mustGetBook(c, "BRL-VIB"), second))

		c.Equal([]uuid.UUID{first.ID, second.ID}, getMessageIDs(c, readTopic(c, topic, 2)))
	})

	t.Run("log unavailable fails within the delivery timeout", func(t *testing.T) {
		c := require.New(t)
		c.NoError(Connect([]string{"localhost:1"}))
		t.Cleanup(Close)

		begin := time.Now()
		c.Error(Publish(context.Background(), newMessageOn(c, "unreachable", "BRL-VIB")))
		c.Less(time.Since(begin), deliveryTimeout+2*time.Second)
	})

	t.Run("cancelled request is not confirmed", func(t *testing.T) {
		c := require.New(t)
		connect(t)
		topic := createTopic(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		c.Error(Publish(ctx, newMessageOn(c, topic, "BRL-VIB")))
	})
}

func newMessage(c *require.Assertions, book string) events.Message {
	message, err := events.NewMessage("producer-test.Test", book, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), map[string]string{})
	c.NoError(err)

	return message
}

func TestMockPublish(t *testing.T) {
	t.Run("message is recorded", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		message := newMessage(c, "BRL-VIB")

		c.NoError(Publish(context.Background(), message))
		c.Equal([]events.Message{message}, mock.Messages)
	})

	t.Run("log unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("broker unreachable")

		c.ErrorIs(Publish(context.Background(), newMessage(c, "BRL-VIB")), mock.Err)
		c.Empty(mock.Messages)
	})

	t.Run("unknown book is not published", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)

		c.ErrorIs(Publish(context.Background(), newMessage(c, "BTC-USD")), books.ErrUnknownBook)
		c.Empty(mock.Messages)
	})
}

func TestPublishInvalidRoute(t *testing.T) {
	c := require.New(t)
	mock := InitMock(t)
	message := newMessage(c, "BRL-VIB")
	message.Route = "no-topic"

	c.ErrorIs(Publish(context.Background(), message), events.ErrInvalidRoute)
	c.Empty(mock.Messages)
}

func TestPublishWithoutConnection(t *testing.T) {
	c := require.New(t)

	c.ErrorIs(Publish(context.Background(), newMessage(c, "BRL-VIB")), ErrNotConnected)
}
