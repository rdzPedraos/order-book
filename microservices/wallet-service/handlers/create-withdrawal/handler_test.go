package createwithdrawal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/identity"
	"github.com/rdzpedraos/order-book/shared/money"
)

func newWithdrawalContext(body, userID string) *gofr.Context {
	req := httptest.NewRequest(http.MethodPost, "/wallet/withdrawals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

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

func withdraw(body, userID string) *httptest.ResponseRecorder {
	data, err := Handle(newWithdrawalContext(body, userID))

	rec := httptest.NewRecorder()
	gofrHTTP.NewResponder(rec, http.MethodPost).Respond(data, err)

	return rec
}

func assertRejected(c *require.Assertions, rec *httptest.ResponseRecorder, mock *walletdb.Mock, status int, code string) {
	c.Equal(status, rec.Code)
	c.Contains(rec.Body.String(), `"code":"`+code+`"`)
	c.Empty(mock.Movements)
}

func TestHandle(t *testing.T) {
	t.Run("withdrawal of BRL", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 10000}}

		rec := withdraw(`{"currency":"BRL","amount":"40.00"}`, "user-a")

		c.Equal(http.StatusCreated, rec.Code)
		c.Len(mock.Movements, 1)
		c.Equal(models.MovementWithdrawal, mock.Movements[0].Type)
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 6000}}, mock.Balances)
		c.Contains(rec.Body.String(), `"type":"WITHDRAWAL","currency":"BRL","amount":"40.00"`)
	})

	t.Run("withdrawal with frozen balance", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 10000, Reserved: 90000}}

		assertRejected(c, withdraw(`{"currency":"BRL","amount":"500.00"}`, "user-a"), mock, http.StatusUnprocessableEntity, "insufficient_funds")
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 10000, Reserved: 90000}}, mock.Balances)
	})

	t.Run("withdrawal of VIB", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.VIB, Available: 10}}

		assertRejected(c, withdraw(`{"currency":"VIB","amount":"1"}`, "user-a"), mock, http.StatusUnprocessableEntity, "currency_not_withdrawable")
	})

	t.Run("invalid amount", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), withdraw(`{"currency":"BRL","amount":"-1.00"}`, "user-a"), mock, http.StatusBadRequest, "invalid_amount")
	})

	t.Run("malformed body", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), withdraw(`[`, "user-a"), mock, http.StatusBadRequest, "invalid_body")
	})

	t.Run("missing user", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), withdraw(`{"currency":"BRL","amount":"1.00"}`, ""), mock, http.StatusUnauthorized, "missing_user_id")
	})

	t.Run("database unavailable", func(t *testing.T) {
		mock := walletdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		assertRejected(require.New(t), withdraw(`{"currency":"BRL","amount":"1.00"}`, "user-a"), mock, http.StatusServiceUnavailable, "service_unavailable")
	})
}
