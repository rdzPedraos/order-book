// Package producer publishes to the exchange's log. A write is confirmed only
// once every in-sync replica has it, a retried write is not duplicated, and
// every message goes to the partition of its book. The producer lives behind a
// package-level variable, set once by Connect at startup, so handlers publish
// without passing it around and tests replace it with InitMock.
package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/fault"
)

// Retries stop after this long, so a request never waits on the log forever.
const deliveryTimeout = 5 * time.Second

var ErrNotConnected = fault.NewWithStatus(http.StatusServiceUnavailable, "log_not_connected", "event log not connected")

type publisher interface {
	publish(ctx context.Context, topic string, book books.Book, message events.Message) error
	publishBatch(ctx context.Context, batch []routedMessage) (int, error)
	close()
}

// A message with the topic and book its route and book field name.
type routedMessage struct {
	topic   string
	book    books.Book
	message events.Message
}

var activePublisher publisher = disconnected{}

func Connect(brokers []string) error {
	producer, err := connectProducer(brokers)
	if err != nil {
		return err
	}

	activePublisher = producer

	return nil
}

// Returns only once the log confirmed the write. The message goes to the topic
// of its route, in the partition of its book, keyed by the book's id.
func Publish(ctx context.Context, message events.Message) error {
	routed, err := routeMessage(message)
	if err != nil {
		return err
	}

	return activePublisher.publish(ctx, routed.topic, routed.book, routed.message)
}

// Sends the messages together and waits for the log once, so a batch costs
// one round trip instead of one per message; within a partition they keep
// their order. Answers how many from the start the log confirmed: on an
// error, the caller retries only the rest, so none is written twice. A
// message that cannot be routed stops the batch before anything is sent.
func PublishBatch(ctx context.Context, messages []events.Message) (int, error) {
	batch := make([]routedMessage, 0, len(messages))

	for _, message := range messages {
		routed, err := routeMessage(message)
		if err != nil {
			return 0, err
		}

		batch = append(batch, routed)
	}

	if len(batch) == 0 {
		return 0, nil
	}

	return activePublisher.publishBatch(ctx, batch)
}

func routeMessage(message events.Message) (routedMessage, error) {
	topic, err := events.GetTopic(message.Route)
	if err != nil {
		return routedMessage{}, err
	}

	book, err := books.Normalize(message.Book)
	if err != nil {
		return routedMessage{}, err
	}

	return routedMessage{topic: topic, book: book, message: message}, nil
}

func Close() {
	activePublisher.close()
	activePublisher = disconnected{}
}

type kafkaProducer struct {
	client *kgo.Client
}

// The producer is idempotent (franz-go's default): the broker drops a retried
// record it already has and keeps the order within the partition.
func connectProducer(brokers []string) (kafkaProducer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RecordPartitioner(kgo.ManualPartitioner()),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordDeliveryTimeout(deliveryTimeout),
	)
	if err != nil {
		return kafkaProducer{}, fmt.Errorf("connect producer: %w", err)
	}

	return kafkaProducer{client: client}, nil
}

func (p kafkaProducer) publish(ctx context.Context, topic string, book books.Book, message events.Message) error {
	_, err := p.publishBatch(ctx, []routedMessage{{topic: topic, book: book, message: message}})

	return err
}

// franz-go reports each record when its write ends, not in the order they
// were given, so each result is placed back by its record.
func (p kafkaProducer) publishBatch(ctx context.Context, batch []routedMessage) (int, error) {
	records := make([]*kgo.Record, 0, len(batch))
	positions := make(map[*kgo.Record]int, len(batch))

	for position, routed := range batch {
		encodedMessage, err := json.Marshal(routed.message)
		if err != nil {
			return 0, fmt.Errorf("encode %s message: %w", routed.message.Route, err)
		}

		record := &kgo.Record{Topic: routed.topic, Partition: routed.book.Partition, Key: []byte(routed.book.ID), Value: encodedMessage}
		records = append(records, record)
		positions[record] = position
	}

	confirmed := len(batch)

	var firstErr error

	for _, result := range p.client.ProduceSync(ctx, records...) {
		if position := positions[result.Record]; result.Err != nil && position < confirmed {
			confirmed, firstErr = position, fmt.Errorf("publish %s: %w", batch[position].message.Route, result.Err)
		}
	}

	return confirmed, firstErr
}

func (p kafkaProducer) close() {
	p.client.Close()
}

type disconnected struct{}

func (disconnected) publish(context.Context, string, books.Book, events.Message) error {
	return ErrNotConnected
}

func (disconnected) publishBatch(context.Context, []routedMessage) (int, error) {
	return 0, ErrNotConnected
}

func (disconnected) close() {}
