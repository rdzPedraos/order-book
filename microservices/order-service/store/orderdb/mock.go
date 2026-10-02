package orderdb

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
)

type Mock struct {
	Orders []models.Order
	Trades []models.Trade
	Err    error
}

// Replaces the database with an in-memory one until the test ends.
func InitMock(t testing.TB) *Mock {
	mock := &Mock{}
	previous := db
	db = mock

	t.Cleanup(func() { db = previous })

	return mock
}

func (m *Mock) insertOrder(_ *gofr.Context, order models.Order) error {
	if m.Err != nil {
		return m.Err
	}

	for _, stored := range m.Orders {
		if stored.ID == order.ID {
			return nil
		}
	}

	m.Orders = append(m.Orders, order)

	return nil
}

func (m *Mock) getOrder(_ *gofr.Context, userID string, id uuid.UUID) (models.Order, error) {
	if m.Err != nil {
		return models.Order{}, m.Err
	}

	for _, order := range m.Orders {
		if order.ID == id && order.UserID == userID {
			return order, nil
		}
	}

	return models.Order{}, models.ErrOrderNotFound
}

func (m *Mock) insertOrUpdateOrder(ctx *gofr.Context, order models.Order) error {
	stored := m.findOrder(order.ID)
	if m.Err != nil || stored == nil {
		return m.insertOrder(ctx, order)
	}

	if stored.Status == models.StatusPending {
		stored.Status, stored.Reason, stored.UpdatedAt = order.Status, order.Reason, order.UpdatedAt
	}

	return nil
}

func (m *Mock) updateCancelledOrder(_ *gofr.Context, id uuid.UUID, reason *string, cancelledAt time.Time) error {
	if m.Err != nil {
		return m.Err
	}

	if stored := m.findOrder(id); stored != nil && !stored.Status.IsFinal() {
		stored.Status, stored.Reason, stored.UpdatedAt = models.StatusCancelled, reason, cancelledAt
	}

	return nil
}

func (m *Mock) updateModifiedOrder(_ *gofr.Context, id uuid.UUID, limit, pendingQuantity int64, modifiedAt time.Time) error {
	if m.Err != nil {
		return m.Err
	}

	if stored := m.findOrder(id); stored != nil && !stored.Status.IsFinal() {
		quantity := stored.FilledQuantity + pendingQuantity
		stored.Limit, stored.Quantity, stored.UpdatedAt = &limit, &quantity, modifiedAt
	}

	return nil
}

func (m *Mock) insertTrade(_ *gofr.Context, trade models.Trade) error {
	if m.Err != nil {
		return m.Err
	}

	for _, recorded := range m.Trades {
		if recorded.ID == trade.ID {
			return nil
		}
	}

	m.Trades = append(m.Trades, trade)

	for _, id := range []uuid.UUID{trade.BuyOrderID, trade.SellOrderID} {
		if stored := m.findOrder(id); stored != nil && !stored.Status.IsFinal() {
			addTrade(stored, trade)
		}
	}

	return nil
}

// The same rule as addTradeToOrder.
func addTrade(order *models.Order, trade models.Trade) {
	order.FilledQuantity += trade.Quantity
	order.FilledAmount += trade.Amount
	order.UpdatedAt = trade.CreatedAt

	isMarketBuy := order.Type == models.TypeMarket && order.Side == models.SideBuy

	switch {
	case isMarketBuy && order.FilledAmount >= *order.Amount, !isMarketBuy && order.FilledQuantity >= *order.Quantity:
		order.Status = models.StatusFilled
	case order.Type == models.TypeLimit:
		order.Status = models.StatusPartiallyFilled
	}
}

func (m *Mock) findOrder(id uuid.UUID) *models.Order {
	for i := range m.Orders {
		if m.Orders[i].ID == id {
			return &m.Orders[i]
		}
	}

	return nil
}

func (m *Mock) listOrders(_ *gofr.Context, query ListQuery) ([]models.Order, error) {
	if m.Err != nil {
		return nil, m.Err
	}

	var found []models.Order

	for _, order := range m.Orders {
		if isInQuery(order, query) {
			found = append(found, order)
		}
	}

	slices.SortFunc(found, func(a, b models.Order) int { return bytes.Compare(b.ID[:], a.ID[:]) })

	return found[:min(len(found), query.Limit)], nil
}

func isInQuery(order models.Order, query ListQuery) bool {
	return order.UserID == query.UserID &&
		(query.Status == nil || order.Status == *query.Status) &&
		(query.Side == nil || order.Side == *query.Side) &&
		(query.Book == nil || order.Book == *query.Book) &&
		(query.Cursor == nil || bytes.Compare(order.ID[:], query.Cursor[:]) < 0)
}
