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
	Reason          *string   `json:"reason"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// Amounts and quantities are decimal strings, in the book's quote and base currency (docs/api.md).
func (o Order) MarshalJSON() ([]byte, error) {
	book, err := books.Normalize(o.Book)
	if err != nil {
		return nil, err
	}

	limit, err := formatOptionalAmount(book.Quote, o.Limit)
	if err != nil {
		return nil, err
	}

	amount, err := formatOptionalAmount(book.Quote, o.Amount)
	if err != nil {
		return nil, err
	}

	averagePrice, err := o.getAveragePrice(book.Base)
	if err != nil {
		return nil, err
	}

	avgPrice, err := formatOptionalAmount(book.Quote, averagePrice)
	if err != nil {
		return nil, err
	}

	quantity, err := formatOptionalAmount(book.Base, o.Quantity)
	if err != nil {
		return nil, err
	}

	filledQuantity, err := money.Format(book.Base, o.FilledQuantity)
	if err != nil {
		return nil, err
	}

	pendingQuantity, err := formatOptionalAmount(book.Base, o.getPendingQuantity())
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
		FilledQuantity:  filledQuantity,
		PendingQuantity: pendingQuantity,
		AvgPrice:        avgPrice,
		Status:          o.Status,
		Reason:          o.Reason,
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
	})
}

// Nil while nothing was executed.
func (o Order) getAveragePrice(base money.Currency) (*int64, error) {
	if o.FilledQuantity == 0 {
		return nil, nil
	}

	average, err := money.AveragePrice(base, o.FilledAmount, o.FilledQuantity)
	if err != nil {
		return nil, err
	}

	return &average, nil
}

// Nil for a market buy by amount.
func (o Order) getPendingQuantity() *int64 {
	if o.Quantity == nil {
		return nil
	}

	pending := *o.Quantity - o.FilledQuantity

	return &pending
}

func formatOptionalAmount(currency money.Currency, amount *int64) (*string, error) {
	if amount == nil {
		return nil, nil
	}

	formatted, err := money.Format(currency, *amount)
	if err != nil {
		return nil, err
	}

	return &formatted, nil
}
