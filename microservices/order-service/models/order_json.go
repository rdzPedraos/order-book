package models

import (
	"encoding/json"
	"time"

	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/money"
)

type orderJSON struct {
	OrderID         string    `json:"orderId"`
	Book            string    `json:"book"`
	Side            Side      `json:"side"`
	Type            OrderType `json:"type"`
	Limit           *string   `json:"limit"`
	Amount          *string   `json:"amount"`
	Quantity        *string   `json:"quantity"`
	FilledQuantity  string    `json:"filledQuantity"`
	PendingQuantity *string   `json:"pendingQuantity"`
	AvgPrice        *string   `json:"avgPrice"`
	Status          Status    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// Amounts and quantities are decimal strings, in the book's quote and base currency (docs/api.md).
func (o Order) MarshalJSON() ([]byte, error) {
	book, err := books.Normalize(o.Book)
	if err != nil {
		return nil, err
	}

	limit, err := formatMoney(book.Quote, o.Limit)
	if err != nil {
		return nil, err
	}

	amount, err := formatMoney(book.Quote, o.Amount)
	if err != nil {
		return nil, err
	}

	avgPrice, err := formatMoney(book.Quote, o.AvgPrice)
	if err != nil {
		return nil, err
	}

	quantity, err := formatMoney(book.Base, o.Quantity)
	if err != nil {
		return nil, err
	}

	filled, err := money.Format(book.Base, o.FilledQuantity)
	if err != nil {
		return nil, err
	}

	pending, err := formatMoney(book.Base, o.pendingQuantity())
	if err != nil {
		return nil, err
	}

	return json.Marshal(orderJSON{
		OrderID:         o.ID.String(),
		Book:            o.Book,
		Side:            o.Side,
		Type:            o.Type,
		Limit:           limit,
		Amount:          amount,
		Quantity:        quantity,
		FilledQuantity:  filled,
		PendingQuantity: pending,
		AvgPrice:        avgPrice,
		Status:          o.Status,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
	})
}

// Nil for a market buy by amount.
func (o Order) pendingQuantity() *int64 {
	if o.Quantity == nil {
		return nil
	}

	pending := *o.Quantity - o.FilledQuantity

	return &pending
}

func formatMoney(c money.Currency, amount *int64) (*string, error) {
	if amount == nil {
		return nil, nil
	}

	formatted, err := money.Format(c, *amount)
	if err != nil {
		return nil, err
	}

	return &formatted, nil
}
