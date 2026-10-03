package walletdb

import (
	"bytes"
	"slices"
	"testing"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/shared/money"
)

type Mock struct {
	Balances  []models.Balance
	Movements []models.Movement
	Err       error
}

// Replaces the database with an in-memory one until the test ends.
func InitMock(t testing.TB) *Mock {
	mock := &Mock{}
	previous := db
	db = mock

	t.Cleanup(func() { db = previous })

	return mock
}

func (m *Mock) getBalances(_ *gofr.Context, userID string) ([]models.Balance, error) {
	if m.Err != nil {
		return nil, m.Err
	}

	var balances []models.Balance

	for _, balance := range m.Balances {
		if balance.UserID == userID {
			balances = append(balances, balance)
		}
	}

	return balances, nil
}

func (m *Mock) deposit(_ *gofr.Context, movement models.Movement) error {
	if m.Err != nil {
		return m.Err
	}

	m.getOrCreateBalance(movement.UserID, movement.Currency).Available += movement.Amount
	m.Movements = append(m.Movements, movement)

	return nil
}

func (m *Mock) withdraw(_ *gofr.Context, movement models.Movement) error {
	if m.Err != nil {
		return m.Err
	}

	balance := m.getOrCreateBalance(movement.UserID, movement.Currency)
	if balance.Available < movement.Amount {
		return models.ErrInsufficientFunds
	}

	balance.Available -= movement.Amount
	m.Movements = append(m.Movements, movement)

	return nil
}

func (m *Mock) listMovements(_ *gofr.Context, query MovementQuery) ([]models.Movement, error) {
	if m.Err != nil {
		return nil, m.Err
	}

	var movements []models.Movement

	for _, movement := range slices.Backward(m.Movements) {
		if isInQuery(movement, query) && len(movements) < query.Limit {
			movements = append(movements, movement)
		}
	}

	return movements, nil
}

func isInQuery(movement models.Movement, query MovementQuery) bool {
	return movement.UserID == query.UserID && movement.Result == models.ResultOK &&
		(query.From == nil || !movement.CreatedAt.Before(*query.From)) &&
		(query.To == nil || movement.CreatedAt.Before(*query.To)) &&
		(query.Cursor == nil || bytes.Compare(movement.ID[:], query.Cursor[:]) < 0)
}

func (m *Mock) applyFundsBatch(_ *gofr.Context, operations []models.FundsOperation) ([]models.FundsResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}

	results := make([]models.FundsResult, 0, len(operations))
	for _, operation := range operations {
		results = append(results, models.FundsResult{
			MessageID: operation.MessageID, Type: operation.Type, Result: m.applyFundsOperation(operation),
		})
	}

	return results, nil
}

func (m *Mock) applyFundsOperation(operation models.FundsOperation) models.Result {
	for _, movement := range m.Movements {
		if movement.Type == operation.Type && movement.MessageID != nil && *movement.MessageID == operation.MessageID {
			return movement.Result
		}
	}

	balance := m.getOrCreateBalance(operation.UserID, operation.Currency)
	result := moveFunds(balance, operation)

	if result != models.ResultReleaseExceedsReservation {
		m.Movements = append(m.Movements, models.Movement{
			ID: uuid.Must(uuid.NewV7()), UserID: operation.UserID, Currency: operation.Currency, Type: operation.Type,
			Amount: operation.Amount, OrderID: &operation.OrderID, MessageID: &operation.MessageID, Result: result,
		})
	}

	return result
}

func moveFunds(balance *models.Balance, operation models.FundsOperation) models.Result {
	if operation.Type == models.MovementReserve && balance.Available < operation.Amount {
		return models.ResultInsufficientFunds
	}

	if operation.Type == models.MovementRelease && balance.Reserved < operation.Amount {
		return models.ResultReleaseExceedsReservation
	}

	sign := int64(1)
	if operation.Type == models.MovementRelease {
		sign = -1
	}

	balance.Available -= sign * operation.Amount
	balance.Reserved += sign * operation.Amount

	return models.ResultOK
}

func (m *Mock) getOrCreateBalance(userID string, currency money.Currency) *models.Balance {
	for i := range m.Balances {
		if m.Balances[i].UserID == userID && m.Balances[i].Currency == currency {
			return &m.Balances[i]
		}
	}

	m.Balances = append(m.Balances, models.Balance{UserID: userID, Currency: currency})

	return &m.Balances[len(m.Balances)-1]
}

func (m *Mock) applyTrade(_ *gofr.Context, trade models.Trade) error {
	if m.Err != nil {
		return m.Err
	}

	for _, movement := range m.Movements {
		if movement.Type == models.MovementTradePaid && *movement.MessageID == trade.MessageID {
			return nil
		}
	}

	movements, err := trade.BuildMovements()
	if err != nil {
		return err
	}

	for _, movement := range movements {
		balance := m.getOrCreateBalance(movement.UserID, movement.Currency)
		if movement.Type == models.MovementTradePaid {
			balance.Reserved -= movement.Amount
		} else {
			balance.Available += movement.Amount
		}
	}

	m.Movements = append(m.Movements, movements...)

	return nil
}

func (m *Mock) applyTrades(ctx *gofr.Context, trades []models.Trade) error {
	for _, trade := range trades {
		if err := m.applyTrade(ctx, trade); err != nil {
			return err
		}
	}

	return nil
}
