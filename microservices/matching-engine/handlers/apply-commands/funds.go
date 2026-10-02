package applycommands

import (
	"time"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
)

const walletRetryDelay = 200 * time.Millisecond

// Retries until the wallet answers, so the engine never moves past a command
// whose funds it does not know yet. It gives up only when the engine stops.
func applyFunds(ctx *gofr.Context, operations []walletclient.Operation) ([]walletclient.Result, error) {
	if len(operations) == 0 {
		return nil, nil
	}

	for {
		results, err := walletclient.ApplyFundsBatch(ctx, operations)
		if err == nil {
			return results, nil
		}

		ctx.Logger.Errorf("wallet did not apply %d operations, retrying: %v", len(operations), err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(walletRetryDelay):
		}
	}
}
