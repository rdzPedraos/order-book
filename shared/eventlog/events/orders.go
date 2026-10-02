package events

import "github.com/google/uuid"

// For a reader of the whole topic, like the matching engine.
const TopicOrderCommands = "orders.commands"

const (
	RouteNewOrder    = "orders.commands.NewOrder"
	RouteModifyOrder = "orders.commands.ModifyOrder"
	RouteCancelOrder = "orders.commands.CancelOrder"
)

// Amounts are in minimal units of the book's currencies, as decimal strings.
type NewOrder struct {
	OrderID  uuid.UUID `json:"orderId"`
	UserID   string    `json:"userId"`
	Side     string    `json:"side"`
	Type     string    `json:"type"`
	Limit    *int64    `json:"limit,string,omitempty"`
	Amount   *int64    `json:"amount,string,omitempty"`
	Quantity *int64    `json:"quantity,string,omitempty"`
}

type ModifyOrder struct {
	OrderID  uuid.UUID `json:"orderId"`
	UserID   string    `json:"userId"`
	Limit    *int64    `json:"limit,string,omitempty"`
	Quantity *int64    `json:"quantity,string,omitempty"`
}

type CancelOrder struct {
	OrderID uuid.UUID `json:"orderId"`
	UserID  string    `json:"userId"`
}
