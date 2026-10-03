// Command matching-engine is the single writer of one book, set by BOOK: it
// applies the commands of the book's partition of orders.commands in log
// order, freezes the funds of each order in wallet-service before it enters
// the book, and publishes what happened to orders.events. The book lives in
// memory.
package main

import (
	"strconv"
	"strings"

	"gofr.dev/pkg/gofr"

	applycommands "github.com/rdzpedraos/order-book/microservices/matching-engine/handlers/apply-commands"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
)

// The most commands applied per batch when COMMAND_BATCH_SIZE is not set. A
// batch is not waited for: the engine takes what is available up to the size,
// so the size only matters while it is behind.
const defaultBatchSize = "500"

func main() {
	app := gofr.New()

	maxBatch, err := strconv.Atoi(app.Config.GetOrDefault("COMMAND_BATCH_SIZE", defaultBatchSize))
	if err != nil || maxBatch < 1 {
		app.Logger().Fatalf("COMMAND_BATCH_SIZE %q is not a positive number", app.Config.Get("COMMAND_BATCH_SIZE"))
	}

	book, err := books.Normalize(app.Config.Get("BOOK"))
	if err != nil {
		app.Logger().Fatalf("BOOK %q: %v", app.Config.Get("BOOK"), err)
	}

	brokers := strings.Split(app.Config.Get("COMMAND_LOG_BROKERS"), ",")

	if err := producer.Connect(brokers); err != nil {
		app.Logger().Fatalf("event log: %v", err)
	}

	// funds:batch lives in wallet-service's funds role.
	app.AddHTTPService("wallet", app.Config.Get("WALLET_FUNDS_URL"))

	// The hook's context ends with the app, which stops the reader. Before
	// reading, the engine learns how far it already published.
	app.OnStart(func(ctx *gofr.Context) error {
		last, err := consumer.GetLastMessage(ctx, brokers, events.TopicOrderEvents, book.Partition)
		if err != nil {
			return err
		}

		if err := applycommands.ResumeAfter(book.ID, last); err != nil {
			return err
		}

		return consumer.StartPartition(ctx, brokers, events.TopicOrderCommands, book.Partition, maxBatch, ctx.Logger, applycommands.Handle)
	})

	app.Run()
}
