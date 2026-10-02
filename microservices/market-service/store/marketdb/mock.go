package marketdb

import (
	"cmp"
	"slices"
	"testing"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
)

type Mock struct {
	Levels []models.Level
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

func (m *Mock) updateLevel(_ *gofr.Context, level models.Level) error {
	if m.Err != nil {
		return m.Err
	}

	m.Levels = slices.DeleteFunc(m.Levels, func(stored models.Level) bool {
		return stored.Book == level.Book && stored.Side == level.Side && stored.Price == level.Price
	})

	if level.Volume > 0 {
		m.Levels = append(m.Levels, level)
	}

	return nil
}

func (m *Mock) listLevels(_ *gofr.Context, book, side string, depth int) ([]models.Level, error) {
	if m.Err != nil {
		return nil, m.Err
	}

	var levels []models.Level

	for _, level := range m.Levels {
		if level.Book == book && level.Side == side {
			levels = append(levels, level)
		}
	}

	slices.SortFunc(levels, func(a, b models.Level) int {
		if side == "BUY" {
			return cmp.Compare(b.Price, a.Price)
		}

		return cmp.Compare(a.Price, b.Price)
	})

	return levels[:min(len(levels), depth)], nil
}
