package driver

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// Measures from sending each order to the engine's first event about it,
// OrderAccepted or OrderRejected. The event can be read before the API's
// answer arrives, so both sides are kept and paired when the run ends.
type Tracker struct {
	startedAt time.Time
	now       func() time.Time

	lock    sync.Mutex
	sentAt  map[uuid.UUID]time.Time
	eventAt map[uuid.UUID]time.Time
}

// Events created before startedAt belong to earlier runs and are left out.
func NewTracker(startedAt time.Time) *Tracker {
	return &Tracker{
		startedAt: startedAt, now: time.Now,
		sentAt: map[uuid.UUID]time.Time{}, eventAt: map[uuid.UUID]time.Time{},
	}
}

func (t *Tracker) AddSent(orderID uuid.UUID, sentAt time.Time) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.sentAt[orderID] = sentAt
}

// The handler of consumer.StartPartition over orders.events.
func (t *Tracker) ApplyRecords(_ context.Context, records []consumer.Record) error {
	readAt := t.now()

	t.lock.Lock()
	defer t.lock.Unlock()

	for _, record := range records {
		orderID, ok := t.getFirstEventOrder(record.Message)
		if ok {
			t.eventAt[orderID] = readAt
		}
	}

	return nil
}

func (t *Tracker) getFirstEventOrder(message events.Message) (uuid.UUID, bool) {
	isFirstEvent := message.Route == events.RouteOrderAccepted || message.Route == events.RouteOrderRejected
	if !isFirstEvent || message.CreatedAt.Before(t.startedAt) {
		return uuid.Nil, false
	}

	var header struct {
		OrderID uuid.UUID `json:"orderId"`
	}
	if err := message.ParsePayload(&header); err != nil {
		return uuid.Nil, false
	}

	_, seen := t.eventAt[header.OrderID]

	return header.OrderID, !seen
}

// Answers the latencies of the orders whose event arrived, and how many are
// still waiting for theirs.
func (t *Tracker) getLatencies() ([]time.Duration, int) {
	t.lock.Lock()
	defer t.lock.Unlock()

	latencies := make([]time.Duration, 0, len(t.sentAt))
	missing := 0

	for orderID, sentAt := range t.sentAt {
		eventAt, ok := t.eventAt[orderID]
		if !ok {
			missing++

			continue
		}

		latencies = append(latencies, eventAt.Sub(sentAt))
	}

	return latencies, missing
}
