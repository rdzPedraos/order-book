package driver

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/shared/eventlog/consumer"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

var runStart = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func eventRecord(c *require.Assertions, route string, orderID uuid.UUID, createdAt time.Time) consumer.Record {
	header := events.EventHeader{OrderID: orderID}

	message, err := events.NewEventMessage(route, "BRL-VIB", uuid.New(), 0, createdAt, events.OrderAccepted{EventHeader: header})
	c.NoError(err)

	return consumer.Record{Message: message}
}

func newClockTracker(now time.Time) *Tracker {
	tracker := NewTracker(runStart)
	tracker.now = func() time.Time { return now }

	return tracker
}

func TestTracker(t *testing.T) {
	t.Run("from sending an order to its first engine event", func(t *testing.T) {
		c := require.New(t)
		tracker := newClockTracker(runStart.Add(30 * time.Millisecond))
		orderID := uuid.New()

		tracker.AddSent(orderID, runStart.Add(10*time.Millisecond))
		c.NoError(tracker.ApplyRecords(context.Background(), []consumer.Record{eventRecord(c, events.RouteOrderAccepted, orderID, runStart)}))

		latencies, missing := tracker.getLatencies()
		c.Equal([]time.Duration{20 * time.Millisecond}, latencies)
		c.Zero(missing)
	})

	t.Run("an event read before its order's answer still counts", func(t *testing.T) {
		c := require.New(t)
		tracker := newClockTracker(runStart.Add(30 * time.Millisecond))
		orderID := uuid.New()

		c.NoError(tracker.ApplyRecords(context.Background(), []consumer.Record{eventRecord(c, events.RouteOrderRejected, orderID, runStart)}))
		tracker.AddSent(orderID, runStart.Add(10*time.Millisecond))

		latencies, _ := tracker.getLatencies()
		c.Equal([]time.Duration{20 * time.Millisecond}, latencies)
	})

	t.Run("only the first event of an order counts", func(t *testing.T) {
		c := require.New(t)
		tracker := newClockTracker(runStart.Add(30 * time.Millisecond))
		orderID := uuid.New()
		tracker.AddSent(orderID, runStart)

		c.NoError(tracker.ApplyRecords(context.Background(), []consumer.Record{
			eventRecord(c, events.RouteOrderAccepted, orderID, runStart),
			eventRecord(c, events.RouteTradeExecuted, orderID, runStart),
		}))
		tracker.now = func() time.Time { return runStart.Add(time.Hour) }
		c.NoError(tracker.ApplyRecords(context.Background(), []consumer.Record{eventRecord(c, events.RouteOrderAccepted, orderID, runStart)}))

		latencies, _ := tracker.getLatencies()
		c.Equal([]time.Duration{30 * time.Millisecond}, latencies)
	})

	t.Run("events of earlier runs are left out", func(t *testing.T) {
		c := require.New(t)
		tracker := newClockTracker(runStart)

		c.NoError(tracker.ApplyRecords(context.Background(), []consumer.Record{eventRecord(c, events.RouteOrderAccepted, uuid.New(), runStart.Add(-time.Minute))}))

		c.Empty(tracker.eventAt)
	})

	t.Run("the engine rate counts the orders with their event, up to the last event", func(t *testing.T) {
		c := require.New(t)
		tracker := newClockTracker(runStart.Add(2 * time.Second))
		first, second := uuid.New(), uuid.New()
		tracker.AddSent(first, runStart)
		tracker.AddSent(second, runStart.Add(time.Second))
		tracker.AddSent(uuid.New(), runStart.Add(time.Second))

		c.NoError(tracker.ApplyRecords(context.Background(), []consumer.Record{
			eventRecord(c, events.RouteOrderAccepted, first, runStart),
			eventRecord(c, events.RouteOrderAccepted, second, runStart),
		}))

		c.InDelta(1.0, tracker.getEngineRate(runStart), 0.0001)
	})

	t.Run("without events the engine rate is zero", func(t *testing.T) {
		c := require.New(t)
		tracker := newClockTracker(runStart)
		tracker.AddSent(uuid.New(), runStart)

		c.Zero(tracker.getEngineRate(runStart))
	})

	t.Run("an order without its event yet is missing", func(t *testing.T) {
		c := require.New(t)
		tracker := newClockTracker(runStart)
		tracker.AddSent(uuid.New(), runStart)

		latencies, missing := tracker.getLatencies()
		c.Empty(latencies)
		c.Equal(1, missing)
	})
}
