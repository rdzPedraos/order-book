package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

func startPartition(c *require.Assertions, ctx context.Context, topic string, maxBatch int,
	handle func(context.Context, []Record) error,
) {
	c.NoError(StartPartition(ctx, []string{brokers}, topic, 0, maxBatch, silentLogger{}, handle))
}

func sendBatchesTo(batches chan []Record) func(context.Context, []Record) error {
	return func(_ context.Context, batch []Record) error {
		batches <- batch

		return nil
	}
}

func waitForBatch(ctx context.Context, c *require.Assertions, batches chan []Record) []Record {
	select {
	case batch := <-batches:
		return batch
	case <-ctx.Done():
		c.Fail("no batch received")

		return nil
	}
}

func getMessageIDs(batch []Record) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(batch))
	for _, record := range batch {
		ids = append(ids, record.Message.ID)
	}

	return ids
}

func TestStartPartition(t *testing.T) {
	t.Run("reads the partition from the beginning, in log order", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		first, second, third := publish(c, topic, "NewOrder"), publish(c, topic, "NewOrder"), publish(c, topic, "CancelOrder")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []Record, 10)
		startPartition(c, ctx, topic, 500, sendBatchesTo(batches))

		batch := waitForBatch(ctx, c, batches)
		c.Equal([]uuid.UUID{first.ID, second.ID, third.ID}, getMessageIDs(batch))
		c.Equal([]int64{0, 1, 2}, []int64{batch[0].Offset, batch[1].Offset, batch[2].Offset})
	})

	t.Run("a batch holds at most maxBatch messages", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		publish(c, topic, "NewOrder")
		publish(c, topic, "NewOrder")
		publish(c, topic, "NewOrder")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []Record, 10)
		startPartition(c, ctx, topic, 2, sendBatchesTo(batches))

		c.Len(waitForBatch(ctx, c, batches), 2)
		c.Len(waitForBatch(ctx, c, batches), 1)
	})

	t.Run("a failed batch is retried, not skipped", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		message := publish(c, topic, "NewOrder")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []Record, 10)
		calls := 0
		startPartition(c, ctx, topic, 500, func(_ context.Context, batch []Record) error {
			calls++
			batches <- batch

			if calls == 1 {
				return errors.New("wallet unavailable")
			}

			return nil
		})

		c.Equal([]uuid.UUID{message.ID}, getMessageIDs(waitForBatch(ctx, c, batches)))
		c.Equal([]uuid.UUID{message.ID}, getMessageIDs(waitForBatch(ctx, c, batches)))
	})

	t.Run("a record that is not a message is left out of its batch", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		publishRaw(c, topic, []byte("not a message"))
		message := publish(c, topic, "NewOrder")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []Record, 10)
		startPartition(c, ctx, topic, 500, sendBatchesTo(batches))

		batch := waitForBatch(ctx, c, batches)
		c.Equal([]uuid.UUID{message.ID}, getMessageIDs(batch))
		c.Equal(int64(1), batch[0].Offset)
	})

	t.Run("messages keep their content", func(t *testing.T) {
		c := require.New(t)
		connectProducer(t)
		topic := createTopic(t)
		message := publish(c, topic, "NewOrder")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		batches := make(chan []Record, 10)
		startPartition(c, ctx, topic, 500, sendBatchesTo(batches))

		received := waitForBatch(ctx, c, batches)[0].Message
		c.Equal(message.Route, received.Route)
		c.Equal(events.SchemaVersion, received.SchemaVersion)
	})
}
