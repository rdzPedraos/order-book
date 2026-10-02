// Package applycommands is the matching engine's handler of a batch of
// orders.commands, read in log order from the book's partition (design D3):
//
//  1. it asks the wallet, in one call, to reserve the funds of every new order;
//  2. it applies each command in order, with the book's next sequence, and
//     collects its events and the funds to release;
//  3. it asks the wallet, in one call, to release them;
//  4. it publishes the events.
//
// Each command is applied once: a message or an order already applied has no
// effect. The wallet and the log are retried until they answer, so a batch
// never stops half applied while the engine runs. After a restart the engine
// rereads the log from the start to rebuild the book, and publishes only the
// events after the last one already in orders.events (design D6).
package applycommands

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// An error is only the engine stopping while it waits for the wallet.
var applyByRoute = map[string]func(*gofr.Context, *batch, *command) error{
	events.RouteNewOrder:    applyNewOrder,
	events.RouteCancelOrder: applyCancelOrder,
	events.RouteModifyOrder: applyModifyOrder,
}

func Handle(ctx *gofr.Context, records []consumer.Record) error {
	current, err := prepareBatch(ctx, records)
	if err != nil {
		return err
	}

	for _, record := range records {
		if err := applyCommand(ctx, current, record); err != nil {
			return err
		}
	}

	if _, err := applyFunds(ctx, current.releases); err != nil {
		return err
	}

	if err := publishEvents(ctx, current.events); err != nil {
		return err
	}

	return nil
}

func applyCommand(ctx *gofr.Context, current *batch, record consumer.Record) error {
	book := orderbook.GetBook(record.Message.Book)
	if book.IsApplied(record.Message.ID) {
		return nil
	}

	if apply, ok := applyByRoute[record.Message.Route]; ok {
		if err := apply(ctx, current, &command{record: record, book: book}); err != nil {
			return err
		}
	}

	book.MarkApplied(record.Message.ID)

	return nil
}
