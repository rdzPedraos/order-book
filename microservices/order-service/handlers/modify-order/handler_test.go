package modifyorder

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

func modifyOrder(id, body, userID string) *httptest.ResponseRecorder {
	data, err := Handle(newOrderContext(http.MethodPatch, id, body, userID))

	return respond(http.MethodPatch, data, err)
}

func TestHandle(t *testing.T) {
	t.Run("valid change not available yet", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"limit":"92.00","quantity":"5"}`, userA)

		c.Equal(http.StatusNotImplemented, rec.Code)
		c.JSONEq(`{"error":{"code":"not_implemented","message":"not implemented yet"}}`, rec.Body.String())
	})

	t.Run("order of another person", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"limit":"92.00"}`, userB)

		c.Equal(http.StatusNotFound, rec.Code)
		c.JSONEq(`{"error":{"code":"order_not_found","message":"order not found"}}`, rec.Body.String())
	})

	t.Run("malformed id", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder("not-a-uuid", `{"limit":"92.00"}`, userA)

		c.Equal(http.StatusNotFound, rec.Code)
		c.JSONEq(`{"error":{"code":"order_not_found","message":"order not found"}}`, rec.Body.String())
	})

	t.Run("missing user", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"limit":"92.00"}`, "")

		c.Equal(http.StatusUnauthorized, rec.Code)
		c.JSONEq(`{"error":{"code":"missing_user_id","message":"missing X-User-ID header"}}`, rec.Body.String())
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		rec := modifyOrder(orderID.String(), `{"limit":"92.00"}`, userA)

		c.Equal(http.StatusServiceUnavailable, rec.Code)
		c.JSONEq(`{"error":{"code":"service_unavailable","message":"service temporarily unavailable"}}`, rec.Body.String())
	})

	t.Run("malformed body", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"limit":`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_body","message":"request body is not valid JSON"}}`, rec.Body.String())
	})

	t.Run("nothing to modify", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_body","message":"limit or quantity is required"}}`, rec.Body.String())
	})

	t.Run("limit with too many decimals", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"limit":"92.001"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit_price","message":"invalid limit price"}}`, rec.Body.String())
	})

	t.Run("fractional quantity", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"quantity":"2.5"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_quantity","message":"invalid quantity"}}`, rec.Body.String())
	})

	t.Run("zero limit", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"limit":"0.00"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit_price","message":"invalid limit price"}}`, rec.Body.String())
	})

	t.Run("zero quantity", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := modifyOrder(orderID.String(), `{"quantity":"0"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_quantity","message":"invalid quantity"}}`, rec.Body.String())
	})
}
