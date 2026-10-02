// Package walletclient is the matching engine's access to wallet-service's
// POST /wallet/internal/funds:batch, through Gofr's HTTP service client.
// Handlers call ApplyFundsBatch directly; tests replace the wallet with
// InitMock, which reserves and releases like the real one.
package walletclient

import (
	"errors"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/shared/money"
)

const (
	TypeReserve = "RESERVE"
	TypeRelease = "RELEASE"

	ResultOK                        = "OK"
	ResultInsufficientFunds         = "insufficient_funds"
	ResultReleaseExceedsReservation = "release_exceeds_reservation"
)

var ErrUnavailable = errors.New("wallet unavailable")

// MessageID is the log message that causes the operation, so the wallet
// applies a repeated message once. Amount is in the currency's minimal units.
type Operation struct {
	Type      string         `json:"type"`
	MessageID uuid.UUID      `json:"messageId"`
	OrderID   uuid.UUID      `json:"orderId"`
	UserID    string         `json:"userId"`
	Currency  money.Currency `json:"currency"`
	Amount    int64          `json:"amount,string"`
}

type Result struct {
	MessageID uuid.UUID `json:"messageId"`
	Type      string    `json:"type"`
	Result    string    `json:"result"`
}

type wallet interface {
	applyFundsBatch(ctx *gofr.Context, operations []Operation) ([]Result, error)
}

type httpWallet struct{}

var activeWallet wallet = httpWallet{}

// Answers one result per operation, in the same order.
func ApplyFundsBatch(ctx *gofr.Context, operations []Operation) ([]Result, error) {
	return activeWallet.applyFundsBatch(ctx, operations)
}
