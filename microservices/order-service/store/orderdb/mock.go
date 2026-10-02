package orderdb

import (
	"bytes"
	"slices"
	"testing"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
)

type Mock struct {
	Orders []models.Order
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
