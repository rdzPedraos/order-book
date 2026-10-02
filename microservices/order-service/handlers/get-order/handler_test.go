package getorder

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/identity"
)

const (
	userA = "user-a"
	userB = "user-b"
)

var (
	orderID   = uuid.MustParse("01923456-7890-7abc-8def-0123456789ab")
	createdAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
)

func ptr[T any](value T) *T {
	return &value
}

func limitOrder() models.Order {
	return models.Order{
		ID: orderID, UserID: userA, Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit,
		Limit: ptr(int64(9000)), Quantity: ptr(int64(10)), Status: models.StatusPending,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func newRequest(method, target, body, userID string) *http.Request {
	var reader io.Reader = http.NoBody
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")

	if userID != "" {
		req.Header.Set(identity.Header, userID)
	}

	var withUser *http.Request

	identity.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { withUser = r })).
		ServeHTTP(httptest.NewRecorder(), req)

	if withUser == nil {
		return req.WithContext(context.Background())
	}

	return withUser
}

func newOrderContext(method, id, body, userID string) *gofr.Context {
	req := newRequest(method, "/orders/"+id, body, userID)
	req = mux.SetURLVars(req, map[string]string{"id": id})

	return &gofr.Context{Context: req.Context(), Request: gofrHTTP.NewRequest(req)}
}

func respond(method string, data any, err error) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	gofrHTTP.NewResponder(rec, method).Respond(data, err)

	return rec
}

const limitOrderJSON = `{
	"orderId": "01923456-7890-7abc-8def-0123456789ab",
	"book": "BRL-VIB",
	"side": "BUY",
	"type": "LIMIT",
	"limit": "90.00",
	"amount": null,
	"quantity": "10",
	"filledQuantity": "0",
	"pendingQuantity": "10",
	"avgPrice": null,
	"status": "PENDING",
	"reason": null,
	"createdAt": "2026-10-01T12:00:00Z",
	"updatedAt": "2026-10-01T12:00:00Z"
}`

func getOrder(id, userID string) *httptest.ResponseRecorder {
	data, err := Handle(newOrderContext(http.MethodGet, id, "", userID))

	return respond(http.MethodGet, data, err)
}

func TestHandle(t *testing.T) {
	t.Run("own order", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := getOrder(orderID.String(), userA)

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(`{"data":`+limitOrderJSON+`}`, rec.Body.String())
	})

	t.Run("order of another person", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := getOrder(orderID.String(), userB)

		c.Equal(http.StatusNotFound, rec.Code)
		c.JSONEq(`{"error":{"code":"order_not_found","message":"order not found"}}`, rec.Body.String())
	})

	t.Run("malformed id", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := getOrder("not-a-uuid", userA)

		c.Equal(http.StatusNotFound, rec.Code)
		c.JSONEq(`{"error":{"code":"order_not_found","message":"order not found"}}`, rec.Body.String())
	})

	t.Run("missing user", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := getOrder(orderID.String(), "")

		c.Equal(http.StatusUnauthorized, rec.Code)
		c.JSONEq(`{"error":{"code":"missing_user_id","message":"missing X-User-ID header"}}`, rec.Body.String())
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		rec := getOrder(orderID.String(), userA)

		c.Equal(http.StatusServiceUnavailable, rec.Code)
		c.JSONEq(`{"error":{"code":"service_unavailable","message":"service temporarily unavailable"}}`, rec.Body.String())
	})
}
