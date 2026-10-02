package applycommands

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rdzpedraos/order-book/microservices/matching-engine/store/walletclient"
	"github.com/rdzpedraos/order-book/shared/money"
)

func reserveOperation(userID string, amount int64) walletclient.Operation {
	return walletclient.Operation{
		Type: walletclient.TypeReserve, MessageID: uuid.New(), OrderID: uuid.New(), UserID: userID, Currency: money.BRL, Amount: amount,
	}
}

func TestApplyFunds(t *testing.T) {
	t.Run("wallet temporarily down", func(t *testing.T) {
		c := require.New(t)
		wallet := walletclient.InitMock(t)
		wallet.Deposit("ana", money.BRL, 1000)
		wallet.FailTimes = 2

		results, err := applyFunds(newContext(t), []walletclient.Operation{reserveOperation("ana", 800)})
		c.NoError(err)
		c.Equal(walletclient.ResultOK, results[0].Result)
		c.Equal(3, wallet.Calls)
		c.Equal(walletclient.Balance{Available: 200, Reserved: 800}, wallet.GetBalance("ana", money.BRL))
	})

	t.Run("the engine stops while waiting for the wallet", func(t *testing.T) {
		c := require.New(t)
		wallet := walletclient.InitMock(t)
		wallet.FailTimes = 1000
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		engineContext := newContextWith(t, ctx)

		_, err := applyFunds(engineContext, []walletclient.Operation{reserveOperation("ana", 1)})
		c.ErrorIs(err, context.Canceled)
	})

	t.Run("no operations, no call", func(t *testing.T) {
		c := require.New(t)
		wallet := walletclient.InitMock(t)

		results, err := applyFunds(newContext(t), nil)
		c.NoError(err)
		c.Empty(results)
		c.Equal(0, wallet.Calls)
	})
}
