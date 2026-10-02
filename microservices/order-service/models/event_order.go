package models

import (
	"time"

	"github.com/rdzpedraos/order-book/shared/eventlog/events"
)

// The projector may read an order's first event, OrderAccepted or
// OrderRejected, before its NewOrder, so it stores the order from the details
// the event carries. createdAt is the event's, which is the command's.
func NewOrderFromDetails(header events.EventHeader, book string, details events.OrderDetails, status Status, createdAt time.Time) Order {
	return Order{
		ID: header.OrderID, UserID: header.UserID, Book: book, Side: Side(details.Side), Type: OrderType(details.Type),
		Limit: details.Limit, Amount: details.Amount, Quantity: details.Quantity,
		Status: status, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}
