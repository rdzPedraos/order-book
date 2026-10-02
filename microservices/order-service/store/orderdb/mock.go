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

func (m *Mock) listOrders(_ *gofr.Context, q ListQuery) ([]models.Order, error) {
	if m.Err != nil {
		return nil, m.Err
	}

	var found []models.Order

	for _, order := range m.Orders {
		if matches(order, q) {
			found = append(found, order)
		}
	}

	slices.SortFunc(found, func(a, b models.Order) int { return bytes.Compare(b.ID[:], a.ID[:]) })

	return found[:min(len(found), q.Limit)], nil
}

func matches(o models.Order, q ListQuery) bool {
	return o.UserID == q.UserID &&
		(q.Status == nil || o.Status == *q.Status) &&
		(q.Side == nil || o.Side == *q.Side) &&
		(q.Book == nil || o.Book == *q.Book) &&
		(q.Cursor == nil || bytes.Compare(o.ID[:], q.Cursor[:]) < 0)
}
