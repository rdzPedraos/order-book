package orderdb

import (
	"time"

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
	insertOrUpdateOrder(ctx *gofr.Context, order models.Order) error
	updateCancelledOrder(ctx *gofr.Context, id uuid.UUID, reason *string, cancelledAt time.Time) error
	updateModifiedOrder(ctx *gofr.Context, id uuid.UUID, limit, pendingQuantity int64, modifiedAt time.Time) error
	insertTrade(ctx *gofr.Context, trade models.Trade) error
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

// Each engine event is one statement whose condition is the change it may
// make, so two projectors, or a command and an event read at the same time,
// can never overwrite each other.

// For an order's first event, OrderAccepted or OrderRejected, which may arrive
// before its NewOrder: inserts the order with its status, or moves a PENDING
// order to it. An order already out of PENDING is left as it is, so a
// repeated event changes nothing.
func InsertOrUpdateOrder(ctx *gofr.Context, order models.Order) error {
	return db.insertOrUpdateOrder(ctx, order)
}

// A final order is left as it is.
func UpdateCancelledOrder(ctx *gofr.Context, id uuid.UUID, reason *string, cancelledAt time.Time) error {
	return db.updateCancelledOrder(ctx, id, reason, cancelledAt)
}

// The engine's quantity is what is still pending, so the order's quantity
// becomes what it already executed plus that. A final order is left as it is.
func UpdateModifiedOrder(ctx *gofr.Context, id uuid.UUID, limit, pendingQuantity int64, modifiedAt time.Time) error {
	return db.updateModifiedOrder(ctx, id, limit, pendingQuantity, modifiedAt)
}

// Records the trade and adds it to its two orders, in one transaction: a
// limit order is PARTIALLY_FILLED until its quantity is executed, and a market
// order stays PENDING until it is FILLED or its remainder is cancelled. A
// trade already recorded changes nothing.
func InsertTrade(ctx *gofr.Context, trade models.Trade) error {
	return db.insertTrade(ctx, trade)
}
