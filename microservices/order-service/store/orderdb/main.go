package orderdb

import (
	"github.com/google/uuid"
	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"gofr.dev/pkg/gofr"
)

type ListQuery struct {
	UserID string
	Status *models.Status
	Side   *models.Side
	Book   *string
	Cursor *uuid.UUID
	Limit  int
}

type orderDB interface {
	insertOrder(ctx *gofr.Context, order models.Order) error
	listOrders(ctx *gofr.Context, query ListQuery) ([]models.Order, error)
	getOrder(ctx *gofr.Context, userID string, id uuid.UUID) (models.Order, error)
}

type postgres struct{}

var db orderDB = postgres{}

// An order that is already stored is left as it is, so a repeated NewOrder has no effect.
func InsertOrder(ctx *gofr.Context, order models.Order) error {
	return db.insertOrder(ctx, order)
}

func ListOrders(ctx *gofr.Context, query ListQuery) ([]models.Order, error) {
	return db.listOrders(ctx, query)
}

func GetOrder(ctx *gofr.Context, userID string, id uuid.UUID) (models.Order, error) {
	return db.getOrder(ctx, userID, id)
}
