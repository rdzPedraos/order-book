package models

import (
	"time"

	"github.com/google/uuid"
)

// One trade of the engine, between a buy and a sell order. Price and Amount
// are in the book's quote currency, Quantity in its base asset.
type Trade struct {
	ID          uuid.UUID
	BuyOrderID  uuid.UUID
	SellOrderID uuid.UUID
	Price       int64
	Quantity    int64
	Amount      int64
	CreatedAt   time.Time
}
