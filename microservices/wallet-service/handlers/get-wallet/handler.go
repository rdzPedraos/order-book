// Package getwallet handles GET /wallet: it answers the balance in BRL and VIB
// of the person behind the request, zero in a currency they never used.
package getwallet

import (
	"fmt"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/identity"
)

func Handle(ctx *gofr.Context) (any, error) {
	userID, err := identity.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	balances, err := walletdb.GetBalances(ctx, userID)
	if err != nil {
		return nil, fault.From(fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err))
	}

	return balances, nil
}
