package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

func TestNewOrderFromDetails(t *testing.T) {
	t.Run("an order stored from its first event", func(t *testing.T) {
		c := require.New(t)
		createdAt := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
		limit, quantity := int64(9000), int64(10)
		header := events.EventHeader{OrderID: uuid.New(), UserID: "ana"}

		order := NewOrderFromDetails(header, "BRL-VIB", events.OrderDetails{Side: "BUY", Type: "LIMIT", Limit: &limit, Quantity: &quantity}, StatusOpen, createdAt)

		c.Equal(Order{
			ID: header.OrderID, UserID: "ana", Book: "BRL-VIB", Side: SideBuy, Type: TypeLimit, Limit: &limit, Quantity: &quantity,
			Status: StatusOpen, CreatedAt: createdAt, UpdatedAt: createdAt,
		}, order)
	})
}
