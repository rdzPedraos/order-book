package models

import (
	"github.com/google/uuid"

	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/money"
)

// Not stored in the ledger: a release that does not fit moves no money.
const ResultReleaseExceedsReservation Result = "release_exceeds_reservation"

var ErrInvalidOperation = fault.New("invalid_operation", "an operation is RESERVE or RELEASE, with its ids, a wallet currency and a positive amount")

// A reservation or a release the matching engine asks for. MessageID is the log
// message that caused it, so a repeated message applies once.
type FundsOperation struct {
	Type      MovementType   `json:"type"`
	MessageID uuid.UUID      `json:"messageId"`
	OrderID   uuid.UUID      `json:"orderId"`
	UserID    string         `json:"userId"`
	Currency  money.Currency `json:"currency"`
	Amount    int64          `json:"amount,string"`
}

type FundsResult struct {
	MessageID uuid.UUID    `json:"messageId"`
	Type      MovementType `json:"type"`
	Result    Result       `json:"result"`
}

func (o FundsOperation) Validate() error {
	isFundsType := o.Type == MovementReserve || o.Type == MovementRelease
	if !isFundsType || o.MessageID == uuid.Nil || o.OrderID == uuid.Nil || o.UserID == "" ||
		!IsWalletCurrency(o.Currency) || o.Amount <= 0 {
		return ErrInvalidOperation
	}

	return nil
}
