package walletdb

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

func fundsOperation(operationType models.MovementType, userID string, amount int64) models.FundsOperation {
	return models.FundsOperation{
		Type: operationType, MessageID: uuid.Must(uuid.NewV7()), OrderID: uuid.Must(uuid.NewV7()),
		UserID: userID, Currency: money.BRL, Amount: amount,
	}
}

func getResults(c *require.Assertions, operations ...models.FundsOperation) []models.Result {
	results, err := ApplyFundsBatch(&gofr.Context{}, operations)
	c.NoError(err)

	values := make([]models.Result, 0, len(results))
	for _, result := range results {
		values = append(values, result.Result)
	}

	return values
}

func TestMockApplyFundsBatch(t *testing.T) {
	t.Run("reservation, repetition and lack of funds", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 1000}}
		reserve, tooMuch := fundsOperation(models.MovementReserve, "user-a", 800), fundsOperation(models.MovementReserve, "user-a", 500)

		c.Equal([]models.Result{models.ResultOK, models.ResultInsufficientFunds}, getResults(c, reserve, tooMuch))
		c.Equal([]models.Result{models.ResultOK}, getResults(c, reserve))
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 200, Reserved: 800}}, mock.Balances)
	})

	t.Run("release, repetition and excess", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Reserved: 800}}
		release, excess := fundsOperation(models.MovementRelease, "user-a", 500), fundsOperation(models.MovementRelease, "user-a", 400)

		c.Equal([]models.Result{models.ResultOK, models.ResultReleaseExceedsReservation}, getResults(c, release, excess))
		c.Equal([]models.Result{models.ResultOK}, getResults(c, release))
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 500, Reserved: 300}}, mock.Balances)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		_, err := ApplyFundsBatch(&gofr.Context{}, []models.FundsOperation{fundsOperation(models.MovementReserve, "user-a", 1)})
		c.ErrorIs(err, mock.Err)
	})
}
