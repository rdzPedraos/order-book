package events

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const TopicOrderEvents = "orders.events"

// What the matching engine publishes about each order.
const (
	RouteOrderAccepted  = "orders.events.OrderAccepted"
	RouteOrderRejected  = "orders.events.OrderRejected"
	RouteOrderCancelled = "orders.events.OrderCancelled"
	RouteOrderModified  = "orders.events.OrderModified"
)

const (
	ReasonInsufficientFunds = "insufficient_funds"
	ReasonNoLiquidity       = "no_liquidity"
	ReasonInvalidAmount     = "invalid_amount"
)

// Every event carries the command that caused it: Sequence is the command's
// sequence in its book, Index the event's position among that command's
// events, and CommandOffset the command's offset in orders.commands, which
// tells the engine how far it already published.
type EventHeader struct {
	Sequence      uint64    `json:"sequence"`
	Index         int       `json:"index"`
	CommandID     uuid.UUID `json:"commandId"`
	CommandOffset int64     `json:"commandOffset"`
	OrderID       uuid.UUID `json:"orderId"`
	UserID        string    `json:"userId"`
}

// The order as it arrived in its NewOrder, on the first event of each order:
// a reader that gets the event before the command can store the whole order.
// Its creation time is the event's createdAt, which is the command's.
type OrderDetails struct {
	Side     string `json:"side"`
	Type     string `json:"type"`
	Limit    *int64 `json:"limit,string,omitempty"`
	Quantity *int64 `json:"quantity,string,omitempty"`
	Amount   *int64 `json:"amount,string,omitempty"`
}

type OrderAccepted struct {
	EventHeader
	OrderDetails
}

type OrderRejected struct {
	EventHeader
	OrderDetails
	Reason string `json:"reason"`
}

// Released is in the quote for a buy and in the base for a sell. Reason is
// empty when the person cancelled, no_liquidity for a market order's remainder.
type OrderCancelled struct {
	EventHeader
	CancelledQuantity int64  `json:"cancelledQuantity,string"`
	Released          int64  `json:"released,string"`
	Reason            string `json:"reason,omitempty"`
}

type OrderModified struct {
	EventHeader
	Limit    *int64 `json:"limit,string,omitempty"`
	Quantity *int64 `json:"quantity,string,omitempty"`
}

// The id comes from the command, which the log keeps unchanged: the same
// command and index always name the same event, so a retried batch or a
// restart publishes the same ids. createdAt is the command's, since the engine
// never reads the clock.
func NewEventMessage(route, book string, commandID uuid.UUID, index int, createdAt time.Time, payload any) (Message, error) {
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("encode %s payload: %w", route, err)
	}

	return Message{
		ID:            uuid.NewSHA1(commandID, []byte(strconv.Itoa(index))),
		Route:         route,
		Book:          book,
		SchemaVersion: SchemaVersion,
		CreatedAt:     createdAt.UTC(),
		Payload:       encodedPayload,
	}, nil
}
