package walletclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/shared/money"
)

func getResults(c *require.Assertions, operations ...Operation) []string {
	results, err := ApplyFundsBatch(&gofr.Context{}, operations)
	c.NoError(err)

	values := make([]string, 0, len(results))
	for _, result := range results {
		values = append(values, result.Result)
	}

	return values
}

func TestMock(t *testing.T) {
	t.Run("reserves, releases and repeats like the wallet", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Deposit("ana", money.BRL, 1000)
		reserve, tooMuch := reserveOperation("ana", 800), reserveOperation("ana", 500)
		release := Operation{Type: TypeRelease, MessageID: reserve.OrderID, OrderID: reserve.OrderID, UserID: "ana", Currency: money.BRL, Amount: 300}

		c.Equal([]string{ResultOK, ResultInsufficientFunds, ResultOK}, getResults(c, reserve, tooMuch, release))
		c.Equal([]string{ResultOK}, getResults(c, reserve))
		c.Equal(Balance{Available: 500, Reserved: 500}, mock.GetBalance("ana", money.BRL))
	})

	t.Run("a release beyond the reserved balance", func(t *testing.T) {
		c := require.New(t)
		InitMock(t)
		release := Operation{Type: TypeRelease, MessageID: reserveOperation("ana", 1).MessageID, UserID: "ana", Currency: money.BRL, Amount: 1}

		c.Equal([]string{ResultReleaseExceedsReservation}, getResults(c, release))
	})

	t.Run("fails the first calls when asked", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.FailTimes = 1

		_, err := ApplyFundsBatch(&gofr.Context{}, []Operation{reserveOperation("ana", 1)})
		c.ErrorIs(err, ErrUnavailable)
		c.Equal([]string{ResultInsufficientFunds}, getResults(c, reserveOperation("ana", 1)))
		c.Equal(2, mock.Calls)
	})

	t.Run("always fails when Err is set", func(t *testing.T) {
		c := require.New(t)
		mock := InitMock(t)
		mock.Err = errors.New("connection refused")

		_, err := ApplyFundsBatch(&gofr.Context{}, []Operation{reserveOperation("ana", 1)})
		c.ErrorIs(err, mock.Err)
	})
}
