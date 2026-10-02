package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/shared/money"
)

type movementJSON struct {
	MovementID uuid.UUID      `json:"movementId"`
	Type       MovementType   `json:"type"`
	Currency   money.Currency `json:"currency"`
	Amount     string         `json:"amount"`
	OrderID    *uuid.UUID     `json:"orderId"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func (m Movement) MarshalJSON() ([]byte, error) {
	amount, err := money.Format(m.Currency, m.Amount)
	if err != nil {
		return nil, err
	}

	return json.Marshal(movementJSON{
		MovementID: m.ID, Type: m.Type, Currency: m.Currency, Amount: amount, OrderID: m.OrderID, CreatedAt: m.CreatedAt,
	})
}
