// Package marketdb is the market-service's access to its PostgreSQL database:
// the levels of each book. Handlers call its functions directly; tests
// replace the database with InitMock.
package marketdb

import (
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
)

type marketDB interface {
	updateLevel(ctx *gofr.Context, level models.Level) error
	listLevels(ctx *gofr.Context, book, side string, depth int) ([]models.Level, error)
}

type postgres struct{}

var db marketDB = postgres{}

// Writes the level as the engine reported it, or deletes it when its volume
// is 0, so a repeated event writes the same thing.
func UpdateLevel(ctx *gofr.Context, level models.Level) error {
	return db.updateLevel(ctx, level)
}

// The best depth levels of one side: the highest buys or the lowest sells.
func ListLevels(ctx *gofr.Context, book, side string, depth int) ([]models.Level, error) {
	return db.listLevels(ctx, book, side, depth)
}
