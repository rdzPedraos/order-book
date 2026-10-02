package listmovements

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/identity"
	"github.com/rdzpedraos/order-book/shared/money"
)

func newListContext(query, userID string) *gofr.Context {
	req := httptest.NewRequest(http.MethodGet, "/wallet/movements"+query, http.NoBody)
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

func listMovements(query, userID string) *httptest.ResponseRecorder {
	data, err := Handle(newListContext(query, userID))

	rec := httptest.NewRecorder()
	gofrHTTP.NewResponder(rec, http.MethodGet).Respond(data, err)

	return rec
}

func movementOn(day int, userID string, result models.Result) models.Movement {
	createdAt := time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC)

	return models.Movement{
		ID: uuid.Must(uuid.NewV7()), UserID: userID, Currency: money.BRL, Type: models.MovementDeposit,
		Amount: int64(day * 100), Result: result, CreatedAt: createdAt,
	}
}

func seedMovements(mock *walletdb.Mock) {
	mock.Movements = []models.Movement{
		movementOn(1, "user-a", models.ResultOK),
		movementOn(2, "user-a", models.ResultOK),
		movementOn(3, "user-a", models.ResultInsufficientFunds),
		movementOn(4, "user-a", models.ResultOK),
		movementOn(5, "user-b", models.ResultOK),
	}
}

func TestHandle(t *testing.T) {
	t.Run("list of movements in a range, newest first", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		seedMovements(mock)

		rec := listMovements("?from=2026-10-02T00:00:00Z&to=2026-10-05T00:00:00Z", "user-a")

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(`{"data":[
			{"movementId":"`+mock.Movements[3].ID.String()+`","type":"DEPOSIT","currency":"BRL","amount":"4.00","orderId":null,"createdAt":"2026-10-04T12:00:00Z"},
			{"movementId":"`+mock.Movements[1].ID.String()+`","type":"DEPOSIT","currency":"BRL","amount":"2.00","orderId":null,"createdAt":"2026-10-02T12:00:00Z"}
		],"metadata":{"nextCursor":null}}`, rec.Body.String())
	})

	t.Run("next page with the cursor", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		seedMovements(mock)

		first := listMovements("?limit=1", "user-a")
		c.Contains(first.Body.String(), `"nextCursor":"`+mock.Movements[3].ID.String()+`"`)

		second := listMovements("?limit=1&cursor="+mock.Movements[3].ID.String(), "user-a")
		c.Contains(second.Body.String(), `"movementId":"`+mock.Movements[1].ID.String()+`"`)
	})

	t.Run("no movements", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := listMovements("", "user-a")

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(`{"data":[],"metadata":{"nextCursor":null}}`, rec.Body.String())
	})

	t.Run("invalid date", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := listMovements("?from=yesterday", "user-a")

		c.Equal(http.StatusBadRequest, rec.Code)
		c.Contains(rec.Body.String(), `"code":"invalid_date"`)
	})

	t.Run("invalid cursor", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := listMovements("?cursor=nope", "user-a")

		c.Equal(http.StatusBadRequest, rec.Code)
		c.Contains(rec.Body.String(), `"code":"invalid_cursor"`)
	})

	t.Run("limit out of range", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := listMovements("?limit=101", "user-a")

		c.Equal(http.StatusBadRequest, rec.Code)
		c.Contains(rec.Body.String(), `"code":"invalid_limit"`)
	})

	t.Run("missing user", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		c.Equal(http.StatusUnauthorized, listMovements("", "").Code)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.Equal(http.StatusServiceUnavailable, listMovements("", "user-a").Code)
	})
}
