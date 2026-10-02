// Package applyfundsbatch handles POST /wallet/internal/funds:batch, the matching
// engine's way to freeze and unfreeze money before and after an order is in
// the book. It applies the RESERVE and RELEASE operations in order, in one
// transaction, and answers the result of each: OK, insufficient_funds or
// release_exceeds_reservation. Each operation carries the id of the log message
// that caused it, so repeating a message moves no funds twice. Amounts are
// decimal strings in the currency's minimal units, as in the log.
package applyfundsbatch

import (
	"fmt"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/fault"
)

var errEmptyBatch = fault.New("empty_batch", "the body needs an operations list with at least one operation")

type request struct {
	Operations []models.FundsOperation `json:"operations"`
}

func Handle(ctx *gofr.Context) (any, error) {
	req, err := parseRequest(ctx)
	if err != nil {
		return nil, err
	}

	results, err := walletdb.ApplyFundsBatch(ctx, req.Operations)
	if err != nil {
		return nil, fault.From(fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err))
	}

	return results, nil
}

func parseRequest(ctx *gofr.Context) (*request, error) {
	var req request
	if err := ctx.Bind(&req); err != nil {
		return nil, fault.ErrInvalidBody
	}

	// A body without the operations list, such as a single operation, arrives empty.
	if len(req.Operations) == 0 {
		return nil, errEmptyBatch
	}

	for _, operation := range req.Operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
	}

	return &req, nil
}
