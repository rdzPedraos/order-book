package listorders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
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

var createdAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

const order3JSON = `{
	"orderId": "00000000-0000-0000-0000-000000000003",
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
	"createdAt": "2026-10-01T12:00:00Z",
	"updatedAt": "2026-10-01T12:00:00Z"
}`

func ptr[T any](value T) *T {
	return &value
}

func orderID(n byte) uuid.UUID {
	return uuid.UUID{15: n}
}

func order(n byte) models.Order {
	return models.Order{
		ID: orderID(n), UserID: userA, Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit,
		Limit: ptr(int64(9000)), Quantity: ptr(int64(10)), Status: models.StatusPending,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func sellOrder(n byte) models.Order {
	o := order(n)
	o.Side = models.SideSell

	return o
}

func orderOf(userID string, n byte) models.Order {
	o := order(n)
	o.UserID = userID

	return o
}

func seedOrders(mock *orderdb.Mock, count byte) {
	for n := byte(1); n <= count; n++ {
		mock.Orders = append(mock.Orders, order(n))
	}
}

func newestFirst(from, to byte) []models.Order {
	var orders []models.Order

	for n := from; n >= to; n-- {
		orders = append(orders, order(n))
	}

	return orders
}

func pageJSON(c *require.Assertions, nextCursor any, orders []models.Order) string {
	body, err := json.Marshal(map[string]any{"data": orders, "metadata": map[string]any{"nextCursor": nextCursor}})
	c.NoError(err)

	return string(body)
}

func newRequest(target, userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)

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

func listOrders(query, userID string) *httptest.ResponseRecorder {
	req := newRequest("/orders"+query, userID)
	ctx := &gofr.Context{Context: req.Context(), Request: gofrHTTP.NewRequest(req)}

	data, err := Handle(ctx)

	rec := httptest.NewRecorder()
	gofrHTTP.NewResponder(rec, http.MethodGet).Respond(data, err)

	return rec
}

func TestHandle(t *testing.T) {
	t.Run("filter by status and side", func(t *testing.T) {
		c := require.New(t)

		mock := orderdb.InitMock(t)
		cancelled := order(2)
		cancelled.Status = models.Status("CANCELLED")
		mock.Orders = []models.Order{sellOrder(1), cancelled, order(3), orderOf(userB, 4)}

		rec := listOrders("?status=PENDING&side=BUY", userA)

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(`{"data":[`+order3JSON+`],"metadata":{"nextCursor":null}}`, rec.Body.String())
	})

	t.Run("default page size", func(t *testing.T) {
		c := require.New(t)

		mock := orderdb.InitMock(t)
		seedOrders(mock, 21)
		mock.Orders = append(mock.Orders, orderOf(userB, 22))

		rec := listOrders("", userA)

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(pageJSON(c, orderID(2).String(), newestFirst(21, 2)), rec.Body.String())
	})

	t.Run("filter by book in lowercase", func(t *testing.T) {
		c := require.New(t)

		mock := orderdb.InitMock(t)
		otherBook := order(2)
		otherBook.Book = "BTC-USD"
		mock.Orders = []models.Order{order(1), otherBook}

		rec := listOrders("?book=brl-vib", userA)

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(pageJSON(c, nil, []models.Order{order(1)}), rec.Body.String())
	})

	t.Run("next page with filters", func(t *testing.T) {
		c := require.New(t)

		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{order(1), sellOrder(2), order(3), order(4), order(5), orderOf(userB, 6)}

		rec := listOrders("?status=PENDING&side=BUY&limit=2&cursor="+orderID(5).String(), userA)

		c.Equal(http.StatusOK, rec.Code)
		c.JSONEq(pageJSON(c, orderID(3).String(), []models.Order{order(4), order(3)}), rec.Body.String())
	})

	t.Run("limit out of range", func(t *testing.T) {
		c := require.New(t)

		orderdb.InitMock(t)

		rec := listOrders("?limit=500", userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit","message":"limit must be between 1 and 100"}}`, rec.Body.String())
	})

	t.Run("negative limit", func(t *testing.T) {
		c := require.New(t)

		orderdb.InitMock(t)

		rec := listOrders("?limit=-1", userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit","message":"limit must be between 1 and 100"}}`, rec.Body.String())
	})

	t.Run("limit not a number", func(t *testing.T) {
		c := require.New(t)

		orderdb.InitMock(t)

		rec := listOrders("?limit=abc", userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit","message":"limit must be between 1 and 100"}}`, rec.Body.String())
	})

	t.Run("unknown status", func(t *testing.T) {
		c := require.New(t)

		orderdb.InitMock(t)

		rec := listOrders("?status=DONE", userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_status","message":"invalid status"}}`, rec.Body.String())
	})

	t.Run("unknown side", func(t *testing.T) {
		c := require.New(t)

		orderdb.InitMock(t)

		rec := listOrders("?side=HOLD", userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_side","message":"side must be BUY or SELL"}}`, rec.Body.String())
	})

	t.Run("cursor not an order id", func(t *testing.T) {
		c := require.New(t)

		orderdb.InitMock(t)

		rec := listOrders("?cursor=abc", userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_cursor","message":"invalid cursor"}}`, rec.Body.String())
	})

	t.Run("unknown book", func(t *testing.T) {
		c := require.New(t)

		orderdb.InitMock(t)

		rec := listOrders("?book=BTC-USD", userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"unknown_book","message":"unknown book"}}`, rec.Body.String())
	})

	t.Run("missing X-User-ID header", func(t *testing.T) {
		c := require.New(t)

		mock := orderdb.InitMock(t)
		mock.Orders = []models.Order{order(1)}

		rec := listOrders("", "")

		c.Equal(http.StatusUnauthorized, rec.Code)
		c.JSONEq(`{"error":{"code":"missing_user_id","message":"missing X-User-ID header"}}`, rec.Body.String())
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)

		mock := orderdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		rec := listOrders("", userA)

		c.Equal(http.StatusServiceUnavailable, rec.Code)
		c.JSONEq(`{"error":{"code":"service_unavailable","message":"service temporarily unavailable"}}`, rec.Body.String())
	})
}
