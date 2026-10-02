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
	close()
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
	topic, err := events.GetTopic(message.Route)
	if err != nil {
		return err
	}

	book, err := books.Normalize(message.Book)
	if err != nil {
		return err
	}

	return activePublisher.publish(ctx, topic, book, message)
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
	encodedMessage, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode %s message: %w", message.Route, err)
	}

	record := &kgo.Record{Topic: topic, Partition: book.Partition, Key: []byte(book.ID), Value: encodedMessage}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("publish %s: %w", message.Route, err)
	}

	return nil
}

func (p kafkaProducer) close() {
	p.client.Close()
}

type disconnected struct{}

func (disconnected) publish(context.Context, string, books.Book, events.Message) error {
	return ErrNotConnected
}

func (disconnected) close() {}
