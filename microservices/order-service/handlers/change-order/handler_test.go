package changeorder

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
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
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
	"createdAt": "2026-10-01T12:00:00Z",
	"updatedAt": "2026-10-01T12:00:00Z"
}`

func changeOrder(id, body, userID string) *httptest.ResponseRecorder {
	data, err := Handle(newOrderContext(http.MethodPost, id, body, userID))

	return respond(http.MethodPost, data, err)
}

func TestHandle(t *testing.T) {
	t.Run("change of limit requested", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"limit":"92.00"}`, userA)

		c.Len(commandLog.Messages, 1)
		message := commandLog.Messages[0]
		c.Equal(events.RouteModifyOrder, message.Route)

		var modifyOrder events.ModifyOrder
		c.NoError(message.ParsePayload(&modifyOrder))
		c.Equal(events.ModifyOrder{OrderID: orderID, UserID: userA, Limit: ptr(int64(9200))}, modifyOrder)

		c.Equal(http.StatusCreated, rec.Code)
		c.JSONEq(`{"data":`+limitOrderJSON+`}`, rec.Body.String())
		c.Equal(limitOrder(), mock.Orders[0])
	})

	t.Run("market order", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		market := limitOrder()
		market.Type, market.Limit, market.Quantity, market.Amount = models.TypeMarket, nil, nil, ptr(int64(50000))
		mock.Orders = []models.Order{market}

		rec := changeOrder(orderID.String(), `{"limit":"92.00"}`, userA)

		c.Equal(http.StatusConflict, rec.Code)
		c.JSONEq(`{"error":{"code":"order_not_modifiable","message":"only limit orders can be modified"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("log unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}
		commandLog.Err = errors.New("broker unreachable")

		rec := changeOrder(orderID.String(), `{"limit":"92.00"}`, userA)

		c.Equal(http.StatusServiceUnavailable, rec.Code)
		c.JSONEq(`{"error":{"code":"service_unavailable","message":"service temporarily unavailable"}}`, rec.Body.String())
	})

	t.Run("order of another person", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"limit":"92.00"}`, userB)

		c.Equal(http.StatusNotFound, rec.Code)
		c.JSONEq(`{"error":{"code":"order_not_found","message":"order not found"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("malformed id", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder("not-a-uuid", `{"limit":"92.00"}`, userA)

		c.Equal(http.StatusNotFound, rec.Code)
		c.JSONEq(`{"error":{"code":"order_not_found","message":"order not found"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("missing user", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"limit":"92.00"}`, "")

		c.Equal(http.StatusUnauthorized, rec.Code)
		c.JSONEq(`{"error":{"code":"missing_user_id","message":"missing X-User-ID header"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Err = errors.New("connection refused")

		rec := changeOrder(orderID.String(), `{"limit":"92.00"}`, userA)

		c.Equal(http.StatusServiceUnavailable, rec.Code)
		c.JSONEq(`{"error":{"code":"service_unavailable","message":"service temporarily unavailable"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("malformed body", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"limit":`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_body","message":"request body is not valid JSON"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("nothing to modify", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_body","message":"limit or quantity is required"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("limit with too many decimals", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"limit":"92.001"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit_price","message":"invalid limit price"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("fractional quantity", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"quantity":"2.5"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_quantity","message":"invalid quantity"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("zero limit", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"limit":"0.00"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_limit_price","message":"invalid limit price"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})

	t.Run("zero quantity", func(t *testing.T) {
		c := require.New(t)
		mock := orderdb.InitMock(t)
		commandLog := producer.InitMock(t)
		mock.Orders = []models.Order{limitOrder()}

		rec := changeOrder(orderID.String(), `{"quantity":"0"}`, userA)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"invalid_quantity","message":"invalid quantity"}}`, rec.Body.String())
		c.Empty(commandLog.Messages)
	})
}
