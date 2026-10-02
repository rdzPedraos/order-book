package applyfundsbatch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/money"
)

var (
	orderID   = uuid.MustParse("01923456-7890-7abc-8def-000000000001")
	reserveID = uuid.MustParse("01923456-7890-7abc-8def-0000000000a1")
	releaseID = uuid.MustParse("01923456-7890-7abc-8def-0000000000b1")
)

func operation(operationType string, messageID uuid.UUID, amount string) string {
	return `{"type":"` + operationType + `","messageId":"` + messageID.String() + `","orderId":"` + orderID.String() +
		`","userId":"user-a","currency":"BRL","amount":"` + amount + `"}`
}

func applyBatch(operations ...string) *httptest.ResponseRecorder {
	body := `{"operations":[` + strings.Join(operations, ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/wallet/internal/funds:batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	data, err := Handle(&gofr.Context{Context: context.Background(), Request: gofrHTTP.NewRequest(req)})

	rec := httptest.NewRecorder()
	gofrHTTP.NewResponder(rec, http.MethodPost).Respond(data, err)

	return rec
}

func result(messageID uuid.UUID, operationType, value string) string {
	return `{"messageId":"` + messageID.String() + `","type":"` + operationType + `","result":"` + value + `"}`
}

func walletWith(mock *walletdb.Mock, available, reserved int64) {
	mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Available: available, Reserved: reserved}}
}

func TestHandle(t *testing.T) {
	t.Run("successful reservation", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		walletWith(mock, 100000, 0)

		rec := applyBatch(operation("RESERVE", reserveID, "80000"))

		c.Equal(http.StatusCreated, rec.Code)
		c.JSONEq(`{"data":[`+result(reserveID, "RESERVE", "OK")+`]}`, rec.Body.String())
		c.Equal(int64(20000), mock.Balances[0].Available)
		c.Equal(int64(80000), mock.Balances[0].Reserved)
	})

	t.Run("reservation without funds", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		walletWith(mock, 50000, 0)

		rec := applyBatch(operation("RESERVE", reserveID, "80000"))

		c.JSONEq(`{"data":[`+result(reserveID, "RESERVE", "insufficient_funds")+`]}`, rec.Body.String())
		c.Equal(int64(50000), mock.Balances[0].Available)
		c.Equal(int64(0), mock.Balances[0].Reserved)
	})

	t.Run("reservation of someone who never deposited", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := applyBatch(operation("RESERVE", reserveID, "1"))

		c.JSONEq(`{"data":[`+result(reserveID, "RESERVE", "insufficient_funds")+`]}`, rec.Body.String())
	})

	t.Run("repeated reservation", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		walletWith(mock, 100000, 0)
		applyBatch(operation("RESERVE", reserveID, "80000"))

		rec := applyBatch(operation("RESERVE", reserveID, "80000"))

		c.JSONEq(`{"data":[`+result(reserveID, "RESERVE", "OK")+`]}`, rec.Body.String())
		c.Equal(int64(80000), mock.Balances[0].Reserved)
	})

	t.Run("release of an open order that is cancelled", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		walletWith(mock, 0, 54000)

		rec := applyBatch(operation("RELEASE", releaseID, "54000"))

		c.JSONEq(`{"data":[`+result(releaseID, "RELEASE", "OK")+`]}`, rec.Body.String())
		c.Equal(int64(54000), mock.Balances[0].Available)
		c.Equal(int64(0), mock.Balances[0].Reserved)
	})

	t.Run("repeated release", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		walletWith(mock, 0, 108000)

		applyBatch(operation("RELEASE", releaseID, "54000"))
		applyBatch(operation("RELEASE", releaseID, "54000"))

		c.Equal(int64(54000), mock.Balances[0].Available)
		c.Equal(int64(54000), mock.Balances[0].Reserved)
	})

	t.Run("release beyond the reserved balance", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		walletWith(mock, 0, 1000)

		rec := applyBatch(operation("RELEASE", releaseID, "1001"))

		c.JSONEq(`{"data":[`+result(releaseID, "RELEASE", "release_exceeds_reservation")+`]}`, rec.Body.String())
		c.Equal(int64(1000), mock.Balances[0].Reserved)
	})

	t.Run("operations apply in order", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		walletWith(mock, 100000, 0)
		secondID := uuid.MustParse("01923456-7890-7abc-8def-0000000000a2")

		rec := applyBatch(operation("RESERVE", reserveID, "80000"), operation("RESERVE", secondID, "50000"))

		c.JSONEq(`{"data":[`+result(reserveID, "RESERVE", "OK")+`,`+result(secondID, "RESERVE", "insufficient_funds")+`]}`, rec.Body.String())
	})

	t.Run("invalid operation", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)

		rec := applyBatch(operation("SETTLE", reserveID, "1"))

		c.Equal(http.StatusBadRequest, rec.Code)
		c.Contains(rec.Body.String(), `"code":"invalid_operation"`)
		c.Empty(mock.Movements)
	})

	t.Run("batch without operations", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)

		rec := applyBatch()

		c.Equal(http.StatusBadRequest, rec.Code)
		c.JSONEq(`{"error":{"code":"empty_batch","message":"the body needs an operations list with at least one operation"}}`, rec.Body.String())
		c.Empty(mock.Movements)
	})

	t.Run("single operation without the operations list", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)
		req := httptest.NewRequest(http.MethodPost, "/wallet/internal/funds:batch", strings.NewReader(operation("RESERVE", reserveID, "5000")))

		_, err := Handle(&gofr.Context{Context: context.Background(), Request: gofrHTTP.NewRequest(req)})
		c.ErrorIs(err, errEmptyBatch)
	})

	t.Run("malformed body", func(t *testing.T) {
		c := require.New(t)
		walletdb.InitMock(t)

		rec := applyBatch(`{`)

		c.Equal(http.StatusBadRequest, rec.Code)
		c.Contains(rec.Body.String(), `"code":"invalid_body"`)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := walletdb.InitMock(t)
		mock.Err = errors.New("connection refused")

		c.Equal(http.StatusServiceUnavailable, applyBatch(operation("RESERVE", reserveID, "1")).Code)
	})
}
