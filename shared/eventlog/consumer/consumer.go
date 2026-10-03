// Package consumer reads the exchange's log in batches: StartGroup reads the
// routes it names, "<topic>.<type>", as a consumer group, and StartPartition
// every message of one partition. One handler applies each batch, and a batch
// is retried until it succeeds, so a failure never skips a message.
package consumer

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// Satisfied by Gofr's logger, so a service passes ctx.Logger as it is.
type Logger interface {
	Errorf(format string, args ...any)
}

const (
	// The broker answers as soon as one record arrives or this time passes,
	// so a message reaches the consumer in milliseconds even with little traffic.
	fetchMaxWait = 100 * time.Millisecond
	retryDelay   = 200 * time.Millisecond
)

// Reads the topics of routes as a consumer group, in the background until ctx
// ends, in batches of what is available up to maxBatch, without waiting for
// more. Only the messages of routes reach applyBatch, in the order read. The
// batch is committed, together with the records of other routes, once
// applyBatch succeeds, and retried until it does, so a failure never skips a
// message.
func StartGroup[C context.Context](ctx C, brokers []string, group string, routes []string, maxBatch int,
	logger Logger, applyBatch func(C, []events.Message) error,
) error {
	topics, err := listTopics(routes)
	if err != nil {
		return err
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxWait(fetchMaxWait),
	)
	if err != nil {
		return fmt.Errorf("connect consumer: %w", err)
	}

	go readGroupBatches(ctx, client, routes, maxBatch, logger, applyBatch)

	return nil
}

func listTopics(routes []string) ([]string, error) {
	topics := make([]string, 0, len(routes))

	for _, route := range routes {
		topic, err := events.GetTopic(route)
		if err != nil {
			return nil, err
		}

		if !slices.Contains(topics, topic) {
			topics = append(topics, topic)
		}
	}

	return topics, nil
}

func readGroupBatches[C context.Context](ctx C, client *kgo.Client, routes []string, maxBatch int, logger Logger,
	applyBatch func(C, []events.Message) error,
) {
	defer client.Close()

	for ctx.Err() == nil {
		fetches := client.PollRecords(ctx, maxBatch)
		fetches.EachError(func(topic string, partition int32, err error) {
			logger.Errorf("fetching %s/%d: %v", topic, partition, err)
		})

		records := fetches.Records()
		if len(records) == 0 {
			continue
		}

		batch := selectMessages(decodeBatch(records, logger), routes)
		if len(batch) > 0 && !retry(ctx, logger, "handling a batch", func() error { return applyBatch(ctx, batch) }) {
			return
		}

		if err := client.CommitRecords(ctx, records...); err != nil {
			logger.Errorf("committing a batch of %d records: %v", len(records), err)
		}
	}
}

func selectMessages(records []Record, routes []string) []events.Message {
	messages := make([]events.Message, 0, len(records))

	for _, record := range records {
		if slices.Contains(routes, record.Message.Route) {
			messages = append(messages, record.Message)
		}
	}

	return messages
}

// Calls apply until it succeeds, and reports whether it did before ctx ended.
func retry(ctx context.Context, logger Logger, what string, apply func() error) bool {
	for {
		err := apply()
		if err == nil {
			return true
		}

		logger.Errorf("%s failed, retrying: %v", what, err)

		select {
		case <-ctx.Done():
			return false
		case <-time.After(retryDelay):
		}
	}
}
