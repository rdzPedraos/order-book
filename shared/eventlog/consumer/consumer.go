// Package consumer reads the exchange's log as a consumer group. Each handler
// subscribes to a route, "<topic>.<type>", like an HTTP route; a record is
// committed only after its handler applied it, so a failure never skips one.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"
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

// Reads the subscribed topics in the background until ctx ends.
func Start[C context.Context](ctx C, brokers []string, group string, logger Logger, subscriptions ...Subscription[C]) error {
	topics, err := getTopics(subscriptions)
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

	go consumeRecords(ctx, client, logger, subscriptions)

	return nil
}

func consumeRecords[C context.Context](ctx C, client *kgo.Client, logger Logger, subscriptions []Subscription[C]) {
	defer client.Close()

	for ctx.Err() == nil {
		fetches := client.PollFetches(ctx)
		fetches.EachError(func(topic string, partition int32, err error) {
			logger.Errorf("fetching %s/%d: %v", topic, partition, err)
		})

		fetches.EachRecord(func(record *kgo.Record) {
			if applyRecord(ctx, record, logger, subscriptions) {
				commitRecord(ctx, client, logger, record)
			}
		})
	}
}

// Reports whether the record is done. A record that is not a message, or that
// nobody subscribed to, is done without handling it, since retrying it would
// fail forever; one whose handler fails is retried until it succeeds or ctx ends.
func applyRecord[C context.Context](ctx C, record *kgo.Record, logger Logger, subscriptions []Subscription[C]) bool {
	var message events.Message
	if err := json.Unmarshal(record.Value, &message); err != nil {
		logger.Errorf("skipping a record of %s at offset %d that is not a message: %v", record.Topic, record.Offset, err)

		return true
	}

	subscription, ok := findSubscription(subscriptions, message)
	if !ok {
		return true
	}

	for {
		err := subscription.handle(ctx, message)
		if err == nil {
			return true
		}

		logger.Errorf("handling %s %s failed, retrying: %v", message.Route, message.ID, err)

		select {
		case <-ctx.Done():
			return false
		case <-time.After(retryDelay):
		}
	}
}

func commitRecord(ctx context.Context, client *kgo.Client, logger Logger, record *kgo.Record) {
	if err := client.CommitRecords(ctx, record); err != nil {
		logger.Errorf("committing %s at offset %d: %v", record.Topic, record.Offset, err)
	}
}
