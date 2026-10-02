package models

import (
	"time"

	"github.com/google/uuid"
)

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type OrderType string

const (
	TypeLimit  OrderType = "LIMIT"
	TypeMarket OrderType = "MARKET"
)

type Status string

const (
	StatusPending Status = "PENDING"
)

func IsValidStatus(s Status) bool {
	return s == StatusPending
}

// Limit and Amount are in the book's quote currency, Quantity and
// FilledQuantity in its base asset. Nil means "not applicable".
type Order struct {
	ID             uuid.UUID
	UserID         string
	Book           string
	Side           Side
	Type           OrderType
	Limit          *int64
	Amount         *int64
	Quantity       *int64
	FilledQuantity int64
	AvgPrice       *int64
	Status         Status
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func IsValidSide(s Side) bool {
	return s == SideBuy || s == SideSell
}

func (o Order) Validate() error {
	if !IsValidSide(o.Side) {
		return ErrInvalidSide
	}

	if !o.isSupported() {
		return ErrUnsupportedOrder
	}

	return o.validateAmounts()
}

// The allowed combinations of docs/api.md: a market buy is by amount; a limit
// order and a market sell are by quantity.
func (o Order) isSupported() bool {
	if o.Type == TypeMarket && o.Side == SideBuy {
		return o.Amount != nil && o.Quantity == nil
	}

	return o.Quantity != nil && o.Amount == nil
}

// A limit, amount or quantity that was sent must be greater than zero.
func (o Order) validateAmounts() error {
	if o.Limit != nil && *o.Limit <= 0 {
		return ErrInvalidLimit
	}

	if o.Amount != nil && *o.Amount <= 0 {
		return ErrInvalidAmount
	}

	if o.Quantity != nil && *o.Quantity <= 0 {
		return ErrInvalidQuantity
	}

	return nil
}
