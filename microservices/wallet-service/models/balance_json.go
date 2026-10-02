package models

import (
	"encoding/json"

	"github.com/rdzpedraos/order-book/shared/money"
)

type balanceJSON struct {
	Currency  money.Currency `json:"currency"`
	Available string         `json:"available"`
	Reserved  string         `json:"reserved"`
	Total     string         `json:"total"`
}

// Amounts travel as decimal strings in the currency's scale.
func (b Balance) MarshalJSON() ([]byte, error) {
	available, err := money.Format(b.Currency, b.Available)
	if err != nil {
		return nil, err
	}

	reserved, err := money.Format(b.Currency, b.Reserved)
	if err != nil {
		return nil, err
	}

	total, err := money.Format(b.Currency, b.GetTotal())
	if err != nil {
		return nil, err
	}

	return json.Marshal(balanceJSON{Currency: b.Currency, Available: available, Reserved: reserved, Total: total})
}
