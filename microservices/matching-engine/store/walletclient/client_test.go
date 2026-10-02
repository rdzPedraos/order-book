package walletclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/container"
	"gofr.dev/pkg/gofr/logging"
	"gofr.dev/pkg/gofr/service"

	"github.com/rdzpedraos/order-book/shared/money"
)

func reserveOperation(userID string, amount int64) Operation {
	return Operation{
		Type: TypeReserve, MessageID: uuid.New(), OrderID: uuid.New(), UserID: userID, Currency: money.BRL, Amount: amount,
	}
}

// A context whose wallet service is the test server.
func newWalletContext(t *testing.T, wallet http.HandlerFunc) *gofr.Context {
	t.Helper()

	server := httptest.NewServer(wallet)
	t.Cleanup(server.Close)

	_, mocks := container.NewMockContainer(t)
	mocks.Metrics.EXPECT().RecordHistogram(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	logger := logging.NewLogger(logging.FATAL)

	return &gofr.Context{Context: context.Background(), Container: &container.Container{
		Logger:   logger,
		Services: map[string]service.HTTP{serviceName: service.NewHTTPService(server.URL, logger, mocks.Metrics)},
	}}
}

func TestApplyFundsBatch(t *testing.T) {
	t.Run("operations are sent and their results read", func(t *testing.T) {
		c := require.New(t)
		operation := reserveOperation("ana", 80000)
		var received map[string][]map[string]any

		ctx := newWalletContext(t, func(w http.ResponseWriter, r *http.Request) {
			c.Equal("/"+fundsBatchPath, r.URL.Path)
			body, _ := io.ReadAll(r.Body)
			c.NoError(json.Unmarshal(body, &received))

			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":[{"messageId":"` + operation.MessageID.String() + `","type":"RESERVE","result":"insufficient_funds"}]}`))
		})

		results, err := ApplyFundsBatch(ctx, []Operation{operation})
		c.NoError(err)
		c.Equal([]Result{{MessageID: operation.MessageID, Type: TypeReserve, Result: ResultInsufficientFunds}}, results)
		c.Equal("80000", received["operations"][0]["amount"])
		c.Equal("ana", received["operations"][0]["userId"])
	})

	t.Run("wallet answers an error", func(t *testing.T) {
		c := require.New(t)
		ctx := newWalletContext(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		_, err := ApplyFundsBatch(ctx, []Operation{reserveOperation("ana", 1)})
		c.ErrorContains(err, "503")
	})

	t.Run("wallet answers something that is not a result", func(t *testing.T) {
		c := require.New(t)
		ctx := newWalletContext(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`not json`))
		})

		_, err := ApplyFundsBatch(ctx, []Operation{reserveOperation("ana", 1)})
		c.ErrorContains(err, "decode")
	})

	t.Run("wallet answers fewer results than operations", func(t *testing.T) {
		c := require.New(t)
		ctx := newWalletContext(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":[]}`))
		})

		_, err := ApplyFundsBatch(ctx, []Operation{reserveOperation("ana", 1)})
		c.ErrorContains(err, "results")
	})

	t.Run("wallet unreachable", func(t *testing.T) {
		c := require.New(t)
		ctx := newWalletContext(t, func(http.ResponseWriter, *http.Request) {})
		ctx.Services[serviceName] = service.NewHTTPService("http://localhost:1", ctx.Logger, nil)

		_, err := ApplyFundsBatch(ctx, []Operation{reserveOperation("ana", 1)})
		c.Error(err)
	})
}
