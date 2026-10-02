package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/shared/money"
)

// What a trade writes in the ledger for each person: what they pay leaves
// their reserved balance, and what they receive enters their available one.
const (
	MovementTradePaid     MovementType = "TRADE_PAID"
	MovementTradeReceived MovementType = "TRADE_RECEIVED"
)

// One trade between two people: the buyer pays Amount of the quote and
// receives Quantity of the base, and the seller the other way round.
// MessageID is the trade's event, so applying it twice moves money once.
type Trade struct {
	MessageID   uuid.UUID
	BuyOrderID  uuid.UUID
	SellOrderID uuid.UUID
	BuyerID     string
	SellerID    string
	Base        money.Currency
	Quote       money.Currency
	Quantity    int64
	Amount      int64
	CreatedAt   time.Time
}

func (t Trade) BuildMovements() ([]Movement, error) {
	movements := []Movement{
		t.buildMovement(MovementTradePaid, t.BuyerID, t.BuyOrderID, t.Quote, t.Amount),
		t.buildMovement(MovementTradeReceived, t.BuyerID, t.BuyOrderID, t.Base, t.Quantity),
		t.buildMovement(MovementTradePaid, t.SellerID, t.SellOrderID, t.Base, t.Quantity),
		t.buildMovement(MovementTradeReceived, t.SellerID, t.SellOrderID, t.Quote, t.Amount),
	}

	for i := range movements {
		movementID, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("generate movement id: %w", err)
		}

		movements[i].ID = movementID
	}

	return movements, nil
}

func (t Trade) buildMovement(movementType MovementType, userID string, orderID uuid.UUID, currency money.Currency, amount int64) Movement {
	return Movement{
		UserID: userID, Currency: currency, Type: movementType, Amount: amount,
		OrderID: &orderID, MessageID: &t.MessageID, Result: ResultOK, CreatedAt: t.CreatedAt,
	}
}
