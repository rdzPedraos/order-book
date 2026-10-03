package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// A message read from a fixed partition, with its offset there.
type Record struct {
	Offset  int64
	Message events.Message
}

// Reads one partition of topic from its first offset, without a consumer
// group, in the background until ctx ends: for a reader that must see every
// message of a partition in order, like the matching engine of a book. Each
// batch is what is available, up to maxBatch, without waiting for more. A
// failed batch is retried until applyBatch succeeds, so the reader never moves
// past it; a record that is not a message is logged and left out of its batch.
func StartPartition[C context.Context](ctx C, brokers []string, topic string, partition int32, maxBatch int,
	logger Logger, applyBatch func(C, []Record) error,
) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: {partition: kgo.NewOffset().AtStart()}}),
		kgo.FetchMaxWait(fetchMaxWait),
	)
	if err != nil {
		return fmt.Errorf("connect partition reader: %w", err)
	}

	go readBatches(ctx, client, maxBatch, logger, applyBatch)

	return nil
}

func readBatches[C context.Context](ctx C, client *kgo.Client, maxBatch int, logger Logger, applyBatch func(C, []Record) error) {
	defer client.Close()

	for ctx.Err() == nil {
		fetches := client.PollRecords(ctx, maxBatch)
		fetches.EachError(func(topic string, partition int32, err error) {
			logger.Errorf("fetching %s/%d: %v", topic, partition, err)
		})

		batch := decodeBatch(fetches.Records(), logger)
		if len(batch) > 0 {
			what := fmt.Sprintf("handling a batch from offset %d", batch[0].Offset)
			retry(ctx, logger, what, func() error { return applyBatch(ctx, batch) })
		}
	}
}

func decodeBatch(records []*kgo.Record, logger Logger) []Record {
	batch := make([]Record, 0, len(records))

	for _, record := range records {
		var message events.Message
		if err := json.Unmarshal(record.Value, &message); err != nil {
			logger.Errorf("skipping a record of %s at offset %d that is not a message: %v", record.Topic, record.Offset, err)

			continue
		}

		batch = append(batch, Record{Offset: record.Offset, Message: message})
	}

	return batch
}
