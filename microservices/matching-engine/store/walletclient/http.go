package walletclient

import (
	"encoding/json"
	"fmt"
	"net/http"

	"gofr.dev/pkg/gofr"
)

// The Gofr HTTP service main.go registers with the funds role's address.
const (
	serviceName    = "wallet"
	fundsBatchPath = "wallet/internal/funds:batch"
)

func (httpWallet) applyFundsBatch(ctx *gofr.Context, operations []Operation) ([]Result, error) {
	body, err := json.Marshal(map[string][]Operation{"operations": operations})
	if err != nil {
		return nil, fmt.Errorf("encode operations: %w", err)
	}

	response, err := ctx.GetHTTPService(serviceName).Post(ctx, fundsBatchPath, nil, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%w: answered %d", ErrUnavailable, response.StatusCode)
	}

	var decoded struct {
		Data []Result `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode results: %w", err)
	}

	if len(decoded.Data) != len(operations) {
		return nil, fmt.Errorf("%w: %d results for %d operations", ErrUnavailable, len(decoded.Data), len(operations))
	}

	return decoded.Data, nil
}
