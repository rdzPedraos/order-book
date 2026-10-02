package applycommands

import (
	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/models"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/orderbook"
	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/money"
)

// What a batch collects while its commands are applied.
type batch struct {
	orders       map[uuid.UUID]*preparedOrder
	reservations map[uuid.UUID]string
	releases     []walletclient.Operation
	events       []events.Message
}

// One command being applied: its events and its trades are numbered in the
// order they are emitted.
type command struct {
	record   consumer.Record
	book     *orderbook.Book
	sequence uint64
	index    int
	fills    int

	touchedLevels []touchedLevel
}

// Reserves, in one call, the funds of every new order of the batch that was
// never applied, before any command is applied.
func prepareBatch(ctx *gofr.Context, records []consumer.Record) (*batch, error) {
	current := &batch{orders: map[uuid.UUID]*preparedOrder{}, reservations: map[uuid.UUID]string{}}

	var reservations []walletclient.Operation

	seenOrders := map[uuid.UUID]bool{}

	for _, record := range records {
		prepared := prepareNewOrder(ctx, record, seenOrders)
		if prepared == nil {
			continue
		}

		current.orders[record.Message.ID] = prepared
		if prepared.err == nil {
			reservations = append(reservations, prepared.reservation(walletclient.TypeReserve, record.Message.ID))
		}
	}

	results, err := applyFunds(ctx, reservations)
	if err != nil {
		return nil, err
	}

	for _, result := range results {
		current.reservations[result.MessageID] = result.Result
	}

	return current, nil
}

// The id of the message that frees the funds makes the release apply once,
// so what one command frees for its order goes in a single release.
func (b *batch) release(messageID uuid.UUID, order models.Order, currency money.Currency, amount int64) {
	if last := len(b.releases) - 1; last >= 0 && b.releases[last].MessageID == messageID {
		b.releases[last].Amount += amount

		return
	}

	b.releases = append(b.releases, walletclient.Operation{
		Type: walletclient.TypeRelease, MessageID: messageID, OrderID: order.ID,
		UserID: order.UserID, Currency: currency, Amount: amount,
	})
}

// createdAt is the command's, since the engine never reads the clock.
func (b *batch) emit(ctx *gofr.Context, current *command, route string, orderID uuid.UUID, userID string,
	buildPayload func(events.EventHeader) any,
) {
	header := events.EventHeader{
		Sequence: current.sequence, Index: current.index, CommandID: current.record.Message.ID,
		CommandOffset: current.record.Offset, OrderID: orderID, UserID: userID,
	}
	current.index++

	if current.book.IsPublished(current.record.Offset, header.Index) {
		return
	}

	message, err := events.NewEventMessage(route, current.record.Message.Book, current.record.Message.ID, header.Index,
		current.record.Message.CreatedAt, buildPayload(header))
	if err != nil {
		ctx.Logger.Errorf("building %s for order %s: %v", route, orderID, err)

		return
	}

	b.events = append(b.events, message)
}
