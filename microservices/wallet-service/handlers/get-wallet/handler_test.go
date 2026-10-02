package getwallet

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/identity"
	"github.com/rdzpedraos/order-book/shared/money"
)

func newWalletContext(userID string) *gofr.Context {
	req := httptest.NewRequest(http.MethodGet, "/wallet", http.NoBody)
	if userID != "" {
		req.Header.Set(identity.Header, userID)
	}

	var withUser *http.Request

	identity.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { withUser = r })).
		ServeHTTP(httptest.NewRecorder(), req)

	if withUser == nil {
		withUser = req.WithContext(context.Background())
	}

	return &gofr.Context{Context: withUser.Context(), Request: gofrHTTP.NewRequest(withUser)}
}

func getWallet(userID string) *httptest.ResponseRecorder {
	data, err := Handle(newWalletContext(userID))

	rec := httptest.NewRecorder()
	gofrHTTP.NewResponder(rec, http.MethodGet).Respond(data, err)

	return rec
}

func TestHandle(t *testing.T) {
	t.Run("balance is read", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{
			{UserID: "user-a", Currency: money.BRL, Available: 10000, Reserved: 90000},
			{UserID: "user-a", Currency: money.VIB, Available: 5},
			{UserID: "user-b", Currency: money.BRL, Available: 777},
		}

		rec := getWallet("user-a")

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(`{"data":[
			{"currency":"BRL","available":"100.00","reserved":"900.00","total":"1000.00"},
			{"currency":"VIB","available":"5","reserved":"0","total":"5"}
		]}`, rec.Body.String())
	})

	t.Run("wallet without movements", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := getWallet("never-operated")

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(`{"data":[
			{"currency":"BRL","available":"0.00","reserved":"0.00","total":"0.00"},
			{"currency":"VIB","available":"0","reserved":"0","total":"0"}
		]}`, rec.Body.String())
	})

	t.Run("missing user", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := getWallet("")

		c.Equal(http.StatusUnauthorized, rec.Code)
		c.JSONEq(`{"error":{"code":"missing_user_id","message":"missing X-User-ID header"}}`, rec.Body.String())
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		rec := getWallet("user-a")

		c.Equal(http.StatusServiceUnavailable, rec.Code)
		c.JSONEq(`{"error":{"code":"service_unavailable","message":"service temporarily unavailable"}}`, rec.Body.String())
	})
}
