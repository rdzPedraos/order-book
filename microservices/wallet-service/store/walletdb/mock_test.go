package walletdb

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

func deposit(userID string, currency money.Currency, amount int64) models.Movement {
	return models.Movement{
		ID: uuid.Must(uuid.NewV7()), UserID: userID, Currency: currency, Type: models.MovementDeposit, Amount: amount,
		Result: models.ResultOK, CreatedAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
	}
}

func TestMockGetBalances(t *testing.T) {
	t.Run("only the person's balances, with every currency", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Balances = []models.Balance{
			{UserID: "user-a", Currency: money.VIB, Available: 5},
			{UserID: "user-b", Currency: money.BRL, Available: 777},
		}

		balances, err := GetBalances(&gofr.Context{}, "user-a")
		c.NoError(err)
		c.Equal([]models.Balance{
			{UserID: "user-a", Currency: money.BRL},
			{UserID: "user-a", Currency: money.VIB, Available: 5},
		}, balances)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		_, err := GetBalances(&gofr.Context{}, "user-a")
		c.ErrorIs(err, mock.Err)
	})
}

func TestMockDeposit(t *testing.T) {
	t.Run("first deposit creates the balance and its movement", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		movement := deposit("user-a", money.BRL, 15000)

		c.NoError(Deposit(&gofr.Context{}, movement))
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 15000}}, mock.Balances)
		c.Equal([]models.Movement{movement}, mock.Movements)
	})

	t.Run("later deposit adds to the balance", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 100, Reserved: 900}}

		c.NoError(Deposit(&gofr.Context{}, deposit("user-a", money.BRL, 50)))
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 150, Reserved: 900}}, mock.Balances)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(Deposit(&gofr.Context{}, deposit("user-a", money.BRL, 50)), mock.Err)
		c.Empty(mock.Movements)
	})
}

func withdrawal(userID string, amount int64) models.Movement {
	movement := deposit(userID, money.BRL, amount)
	movement.Type = models.MovementWithdrawal

	return movement
}

func TestMockWithdraw(t *testing.T) {
	t.Run("withdrawal takes from available", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 100, Reserved: 900}}
		movement := withdrawal("user-a", 40)

		c.NoError(Withdraw(&gofr.Context{}, movement))
		c.Equal([]models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 60, Reserved: 900}}, mock.Balances)
		c.Equal([]models.Movement{movement}, mock.Movements)
	})

	t.Run("more than available", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Balances = []models.Balance{{UserID: "user-a", Currency: money.BRL, Available: 100, Reserved: 900}}

		c.ErrorIs(Withdraw(&gofr.Context{}, withdrawal("user-a", 500)), models.ErrInsufficientFunds)
		c.Equal(int64(100), mock.Balances[0].Available)
		c.Empty(mock.Movements)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		c.ErrorIs(Withdraw(&gofr.Context{}, withdrawal("user-a", 1)), mock.Err)
	})
}

func TestMockListMovements(t *testing.T) {
	t.Run("own movements that moved money, newest first, up to the limit", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		older, newer, rejected := deposit("user-a", money.BRL, 1), deposit("user-a", money.BRL, 2), deposit("user-a", money.BRL, 3)
		rejected.Result = models.ResultInsufficientFunds
		mock.Movements = []models.Movement{older, newer, rejected, deposit("user-b", money.BRL, 4)}

		movements, err := ListMovements(&gofr.Context{}, MovementQuery{UserID: "user-a", Limit: 10})
		c.NoError(err)
		c.Equal([]models.Movement{newer, older}, movements)

		page, err := ListMovements(&gofr.Context{}, MovementQuery{UserID: "user-a", Cursor: &newer.ID, Limit: 10})
		c.NoError(err)
		c.Equal([]models.Movement{older}, page)
	})

	t.Run("date range", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		movement := deposit("user-a", money.BRL, 1)
		mock.Movements = []models.Movement{movement}
		after, before := movement.CreatedAt.Add(time.Second), movement.CreatedAt

		inRange, err := ListMovements(&gofr.Context{}, MovementQuery{UserID: "user-a", From: &before, To: &after, Limit: 10})
		c.NoError(err)
		c.Len(inRange, 1)

		outOfRange, err := ListMovements(&gofr.Context{}, MovementQuery{UserID: "user-a", From: &after, Limit: 10})
		c.NoError(err)
		c.Empty(outOfRange)

		beforeRange, err := ListMovements(&gofr.Context{}, MovementQuery{UserID: "user-a", To: &before, Limit: 10})
		c.NoError(err)
		c.Empty(beforeRange)
	})

	t.Run("limit", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Movements = []models.Movement{deposit("user-a", money.BRL, 1), deposit("user-a", money.BRL, 2)}

		movements, err := ListMovements(&gofr.Context{}, MovementQuery{UserID: "user-a", Limit: 1})
		c.NoError(err)
		c.Len(movements, 1)
	})

	t.Run("database unavailable", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		_, err := ListMovements(&gofr.Context{}, MovementQuery{UserID: "user-a", Limit: 1})
		c.ErrorIs(err, mock.Err)
	})
}
