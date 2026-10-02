package walletclient

import (
	"testing"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/shared/money"
)

type Balance struct {
	Available int64
	Reserved  int64
}

// FailTimes fails that many calls before answering; Err fails every call.
type Mock struct {
	FailTimes int
	Err       error
	Calls     int
	balances  map[string]*Balance
	results   map[string]string
}

// Replaces the wallet with an in-memory one until the test ends.
func InitMock(t testing.TB) *Mock {
	mock := &Mock{balances: map[string]*Balance{}, results: map[string]string{}}
	previous := activeWallet
	activeWallet = mock

	t.Cleanup(func() { activeWallet = previous })

	return mock
}

func (m *Mock) Deposit(userID string, currency money.Currency, amount int64) {
	m.getBalance(userID, currency).Available += amount
}

func (m *Mock) GetBalance(userID string, currency money.Currency) Balance {
	return *m.getBalance(userID, currency)
}

func (m *Mock) applyFundsBatch(_ *gofr.Context, operations []Operation) ([]Result, error) {
	m.Calls++

	if m.Err != nil {
		return nil, m.Err
	}

	if m.FailTimes > 0 {
		m.FailTimes--

		return nil, ErrUnavailable
	}

	results := make([]Result, 0, len(operations))
	for _, operation := range operations {
		results = append(results, Result{MessageID: operation.MessageID, Type: operation.Type, Result: m.apply(operation)})
	}

	return results, nil
}

// A repeated message answers its first result, as the wallet's ledger does.
func (m *Mock) apply(operation Operation) string {
	key := operation.Type + "/" + operation.MessageID.String()
	if result, ok := m.results[key]; ok {
		return result
	}

	result := m.move(operation)
	if result != ResultReleaseExceedsReservation {
		m.results[key] = result
	}

	return result
}

func (m *Mock) move(operation Operation) string {
	balance := m.getBalance(operation.UserID, operation.Currency)

	if operation.Type == TypeReserve {
		if balance.Available < operation.Amount {
			return ResultInsufficientFunds
		}

		balance.Available -= operation.Amount
		balance.Reserved += operation.Amount

		return ResultOK
	}

	if balance.Reserved < operation.Amount {
		return ResultReleaseExceedsReservation
	}

	balance.Reserved -= operation.Amount
	balance.Available += operation.Amount

	return ResultOK
}

func (m *Mock) getBalance(userID string, currency money.Currency) *Balance {
	key := userID + "/" + string(currency)
	if _, ok := m.balances[key]; !ok {
		m.balances[key] = &Balance{}
	}

	return m.balances[key]
}
