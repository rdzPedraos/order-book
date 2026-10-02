package createorder

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/identity"
)

const userA = "user-a"

const limitOrderBody = `{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":"10"}`

func ptr[T any](value T) *T {
	return &value
}

func getNewOrder(c *require.Assertions, message events.Message) events.NewOrder {
	var newOrder events.NewOrder
	c.NoError(message.ParsePayload(&newOrder))

	return newOrder
}

func assertCreated(c *require.Assertions, rec *httptest.ResponseRecorder, mock *producer.Mock, want models.Order) {
	c.Len(mock.Messages, 1)

	message := mock.Messages[0]
	newOrder := getNewOrder(c, message)
	want.ID, want.CreatedAt, want.UpdatedAt = newOrder.OrderID, message.CreatedAt, message.CreatedAt

	body, err := json.Marshal(map[string]any{"data": want})
	c.NoError(err)

	c.Equal(http.StatusCreated, rec.Code)
	c.JSONEq(string(body), rec.Body.String())

	c.Equal(events.RouteNewOrder, message.Route)
	c.Equal(want.Book, message.Book)
	c.Equal(events.NewOrder{
		OrderID: want.ID, UserID: want.UserID, Side: string(want.Side), Type: string(want.Type),
		Limit: want.Limit, Amount: want.Amount, Quantity: want.Quantity,
	}, newOrder)
}

func limitOrder() models.Order {
	return models.Order{
		UserID: userA, Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeLimit,
		Limit: ptr(int64(9000)), Quantity: ptr(int64(10)), Status: models.StatusPending,
	}
}

func newRequest(body, userID string) *http.Request {
	var reader io.Reader = http.NoBody
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(http.MethodPost, "/orders", reader)
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

func createOrderAs(body, userID string) *httptest.ResponseRecorder {
	req := newRequest(body, userID)
	data, err := Handle(&gofr.Context{Context: req.Context(), Request: gofrHTTP.NewRequest(req)})

	rec := httptest.NewRecorder()
	gofrHTTP.NewResponder(rec, http.MethodPost).Respond(data, err)

	return rec
}

func createOrder(body string) *httptest.ResponseRecorder {
	return createOrderAs(body, userA)
}

func TestHandle(t *testing.T) {
	t.Run("order with its command", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(limitOrderBody)

		assertCreated(c, rec, mock, limitOrder())
	})

	t.Run("market buy by amount", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"BUY","amount":"500.00"}`)

		assertCreated(c, rec, mock, models.Order{
			UserID: userA, Book: "BRL-VIB", Side: models.SideBuy, Type: models.TypeMarket,
			Amount: ptr(int64(50000)), Status: models.StatusPending,
		})
	})

	t.Run("book in lowercase", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"brl-vib","side":"BUY","limit":"90.00","quantity":"10"}`)

		assertCreated(c, rec, mock, limitOrder())
	})

	t.Run("repeated submission", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		first := createOrder(limitOrderBody)
		second := createOrder(limitOrderBody)

		c.Equal(http.StatusCreated, first.Code)
		c.Equal(http.StatusCreated, second.Code)
		c.Len(mock.Messages, 2)
		c.NotEqual(getNewOrder(c, mock.Messages[0]).OrderID, getNewOrder(c, mock.Messages[1]).OrderID)
	})

	t.Run("missing user id", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrderAs(limitOrderBody, "")

		c.Equal(http.StatusUnauthorized, rec.Code)
		c.JSONEq(`{"error":{"code":"missing_user_id","message":"missing X-User-ID header"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("malformed body", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_body","message":"request body is not valid JSON"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("unknown book", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BTC-USD","side":"BUY","limit":"90.00","quantity":"10"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"unknown_book","message":"unknown book"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("tickers in another order", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"VIB-BRL","side":"BUY","limit":"90.00","quantity":"10"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"unknown_book","message":"unknown book"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("limit with too many decimals", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"BUY","limit":"90.001","quantity":"10"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit_price","message":"invalid limit price"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("zero limit", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"BUY","limit":"0","quantity":"10"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit_price","message":"invalid limit price"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("limit with zero quantity", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":"0"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_quantity","message":"invalid quantity"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("market buy with zero amount", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"BUY","amount":"0.00"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_amount","message":"invalid amount"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("amount with too many decimals", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"BUY","amount":"500.001"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_amount","message":"invalid amount"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("fractional quantity", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"BUY","limit":"90.00","quantity":"2.5"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_quantity","message":"invalid quantity"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("invalid order is not stored", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)

		rec := createOrder(`{"book":"BRL-VIB","side":"HOLD","limit":"90.00","quantity":"10"}`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_side","message":"side must be BUY or SELL"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})

	t.Run("log unavailable on create", func(t *testing.T) {
		c := require.New(t)

		mock := producer.InitMock(t)
		mock.Err = errors.New("broker unreachable")

		rec := createOrder(limitOrderBody)

		c.Equal(http.StatusServiceUnavailable, rec.Code)
		c.JSONEq(`{"error":{"code":"service_unavailable","message":"service temporarily unavailable"}}`, rec.Body.String())
		c.Empty(mock.Messages)
	})
}
