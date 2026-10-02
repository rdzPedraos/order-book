package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/shared/books"
)

var createdAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func ptr(v int64) *int64 { return &v }

func TestOrderMarshalJSON(t *testing.T) {
	t.Run("limit order", func(t *testing.T) {
		c := require.New(t)

		body, err := json.Marshal(Order{
			ID: uuid.MustParse("01923456-7890-7abc-8def-0123456789ab"), UserID: "user-a", Book: "BRL-VIB",
			Side: SideBuy, Type: TypeLimit, Limit: ptr(9000), Quantity: ptr(10), FilledQuantity: 4,
			FilledAmount: 35800, Status: StatusPartiallyFilled, CreatedAt: createdAt, UpdatedAt: createdAt,
		})
		c.NoError(err)
		c.JSONEq(`{
			"orderId": "01923456-7890-7abc-8def-0123456789ab",
			"book": "BRL-VIB",
			"side": "BUY",
			"type": "LIMIT",
			"limit": "90.00",
			"amount": null,
			"quantity": "10",
			"filledQuantity": "4",
			"pendingQuantity": "6",
			"avgPrice": "89.50",
			"status": "PARTIALLY_FILLED",
			"reason": null,
			"createdAt": "2026-10-01T12:00:00Z",
			"updatedAt": "2026-10-01T12:00:00Z"
		}`, string(body))
	})

	t.Run("market buy by amount", func(t *testing.T) {
		c := require.New(t)

		body, err := json.Marshal(Order{
			ID: uuid.MustParse("01923456-7890-7abc-8def-0123456789ab"), Book: "BRL-VIB",
			Side: SideBuy, Type: TypeMarket, Amount: ptr(50000), Status: StatusPending,
			CreatedAt: createdAt, UpdatedAt: createdAt,
		})
		c.NoError(err)
		c.JSONEq(`{
			"orderId": "01923456-7890-7abc-8def-0123456789ab",
			"book": "BRL-VIB",
			"side": "BUY",
			"type": "MARKET",
			"limit": null,
			"amount": "500.00",
			"quantity": null,
			"filledQuantity": "0",
			"pendingQuantity": null,
			"avgPrice": null,
			"status": "PENDING",
			"reason": null,
			"createdAt": "2026-10-01T12:00:00Z",
			"updatedAt": "2026-10-01T12:00:00Z"
		}`, string(body))
	})

	t.Run("a rejected order shows the engine's reason", func(t *testing.T) {
		c := require.New(t)
		reason := "insufficient_funds"

		body, err := json.Marshal(Order{
			ID: uuid.MustParse("01923456-7890-7abc-8def-0123456789ab"), Book: "BRL-VIB",
			Side: SideBuy, Type: TypeLimit, Limit: ptr(9000), Quantity: ptr(10), Status: StatusRejected, Reason: &reason,
			CreatedAt: createdAt, UpdatedAt: createdAt,
		})
		c.NoError(err)

		var decoded map[string]any
		c.NoError(json.Unmarshal(body, &decoded))
		c.Equal("REJECTED", decoded["status"])
		c.Equal("insufficient_funds", decoded["reason"])
	})

	t.Run("stored order with an unknown book", func(t *testing.T) {
		c := require.New(t)

		_, err := json.Marshal(Order{Book: "BTC-USD"})
		c.ErrorIs(err, books.ErrUnknownBook)
	})
}
