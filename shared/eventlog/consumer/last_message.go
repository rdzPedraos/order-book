package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

const lastMessageTimeout = 10 * time.Second

// The last message of one partition of topic, or nil when the partition is
// empty. A reader that republishes what it reads uses it to know where it
// stopped, like the matching engine after a restart.
func GetLastMessage(ctx context.Context, brokers []string, topic string, partition int32) (*Record, error) {
	end, err := getEndOffset(ctx, brokers, topic, partition)
	if err != nil || end == 0 {
		return nil, err
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: {partition: kgo.NewOffset().At(end - 1)}}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect last message reader: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, lastMessageTimeout)
	defer cancel()

	fetches := client.PollRecords(ctx, 1)
	if err := fetches.Err(); err != nil {
		return nil, fmt.Errorf("read last message of %s/%d: %w", topic, partition, err)
	}

	record := fetches.Records()[0]

	var message events.Message
	if err := json.Unmarshal(record.Value, &message); err != nil {
		return nil, fmt.Errorf("decode last message of %s/%d: %w", topic, partition, err)
	}

	return &Record{Offset: record.Offset, Message: message}, nil
}

func getEndOffset(ctx context.Context, brokers []string, topic string, partition int32) (int64, error) {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return 0, fmt.Errorf("connect offsets reader: %w", err)
	}
	defer client.Close()

	offsets, err := kadm.NewClient(client).ListEndOffsets(ctx, topic)
	if err != nil {
		return 0, fmt.Errorf("list end offsets of %s: %w", topic, err)
	}

	offset, ok := offsets.Lookup(topic, partition)
	if !ok {
		return 0, fmt.Errorf("no partition %d in %s", partition, topic)
	}

	return offset.Offset, offset.Err
}
