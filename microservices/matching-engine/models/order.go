package models

import "github.com/google/uuid"

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

// An order in the engine. Price is in the quote's minimal units and Quantity,
// what is still pending, in the base's; Reserved is what is still frozen in
// the wallet for it, in the quote for a buy and in the base for a sell.
// Sequence is the order of arrival in its book, which breaks ties between
// orders of the same price.
type Order struct {
	ID       uuid.UUID
	UserID   string
	Side     Side
	Type     OrderType
	Price    int64
	Quantity int64
	Reserved int64
	Sequence uint64
}
