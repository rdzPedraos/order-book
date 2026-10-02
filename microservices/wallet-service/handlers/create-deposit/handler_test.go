package createdeposit

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

func newDepositContext(body, userID string) *gofr.Context {
	req := httptest.NewRequest(http.MethodPost, "/wallet/deposits", strings.NewReader(body))
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

func deposit(body, userID string) *httptest.ResponseRecorder {
	data, err := Handle(newDepositContext(body, userID))

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
	t.Run("deposit of BRL", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)

		rec := deposit(`{"currency":"BRL","amount":"150.00"}`, "user-a")

		c.Equal(http.StatusCreated, rec.Code)
		c.Len(mock.Movements, 1)
		movement := mock.Movements[0]
		c.Equal(models.MovementDeposit, movement.Type)
		c.Equal(int64(15000), movement.Amount)
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 15000}}, mock.Balances)
		c.JSONEq(`{"data":{"movementId":"`+movement.ID.String()+`","type":"DEPOSIT","currency":"BRL","amount":"150.00",
			"orderId":null,"createdAt":"`+movement.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")+`"}}`, rec.Body.String())
	})

	t.Run("deposit of VIB", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)

		rec := deposit(`{"currency":"vib","amount":"10"}`, "user-a")

		c.Equal(http.StatusCreated, rec.Code)
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.VIB, Available: 10}}, mock.Balances)
	})

	t.Run("fractional VIB", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), deposit(`{"currency":"VIB","amount":"1.5"}`, "user-a"), mock, http.StatusBadRequest, "invalid_amount")
	})

	t.Run("zero amount", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), deposit(`{"currency":"BRL","amount":"0"}`, "user-a"), mock, http.StatusBadRequest, "invalid_amount")
	})

	t.Run("missing amount", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), deposit(`{"currency":"BRL"}`, "user-a"), mock, http.StatusBadRequest, "invalid_amount")
	})

	t.Run("unknown currency", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), deposit(`{"currency":"USD","amount":"1.00"}`, "user-a"), mock, http.StatusBadRequest, "invalid_currency")
	})

	t.Run("malformed body", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), deposit(`{"currency":`, "user-a"), mock, http.StatusBadRequest, "invalid_body")
	})

	t.Run("missing user", func(t *testing.T) {
		mock := walletdb.InitMock(t)

		assertRejected(require.New(t), deposit(`{"currency":"BRL","amount":"1.00"}`, ""), mock, http.StatusUnauthorized, "missing_user_id")
	})

	t.Run("database unavailable", func(t *testing.T) {
		mock := walletdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		assertRejected(require.New(t), deposit(`{"currency":"BRL","amount":"1.00"}`, "user-a"), mock, http.StatusServiceUnavailable, "service_unavailable")
	})
}
