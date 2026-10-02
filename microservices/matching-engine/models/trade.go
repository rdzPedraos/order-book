package models

import "github.com/google/uuid"

// One cross between an incoming order and a resting one (the maker), at the
// maker's price. Amount is what the buyer pays, in the quote's minimal units.
type Trade struct {
	BuyOrderID  uuid.UUID
	SellOrderID uuid.UUID
	BuyerID     string
	SellerID    string
	MakerSide   Side
	Price       int64
	Quantity    int64
	Amount      int64
}
