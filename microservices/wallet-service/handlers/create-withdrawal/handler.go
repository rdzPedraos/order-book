// Package createwithdrawal handles POST /wallet/withdrawals: it takes a
// positive amount of BRL from the available balance of the person behind the
// request, never from what is reserved for orders, and answers the movement
// written in the ledger. Only BRL can be withdrawn. Withdrawals are simulated
// and not idempotent: a retried request withdraws twice.
package createwithdrawal

import (
	"errors"
	"fmt"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/identity"
)

type request struct {
	UserID   string  `json:"-"`
	Currency string  `json:"currency"`
	Amount   *string `json:"amount"`
}

func Handle(ctx *gofr.Context) (any, error) {
	req, err := parseRequest(ctx)
	if err != nil {
		return nil, err
	}

	movement, err := models.NewMovement(models.MovementWithdrawal, req.UserID, req.Currency, req.Amount)
	if err != nil {
		return nil, err
	}

	err = walletdb.Withdraw(ctx, *movement)
	if errors.Is(err, models.ErrInsufficientFunds) {
		return nil, err
	}

	if err != nil {
		return nil, fault.From(fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err))
	}

	return movement, nil
}

func parseRequest(ctx *gofr.Context) (*request, error) {
	var req request

	err := ctx.Bind(&req)
	if err != nil {
		return nil, fault.ErrInvalidBody
	}

	req.UserID, err = identity.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	return &req, nil
}
