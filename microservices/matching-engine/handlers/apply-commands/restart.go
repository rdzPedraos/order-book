package applycommands

import (
	"fmt"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// Called once at startup with the last event of the book in orders.events, or
// nil when there is none. Every event carries its command's offset and its
// index, which mark how far the engine already published.
func ResumeAfter(bookID string, last *consumer.Record) error {
	if last == nil {
		return nil
	}

	var header events.EventHeader
	if err := last.Message.ParsePayload(&header); err != nil {
		return fmt.Errorf("read the last published event: %w", err)
	}

	orderbook.GetBook(bookID).SetPublishedUpTo(header.CommandOffset, header.Index)

	return nil
}
