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

// What one message of the log changes in the orders.
type ChangeKind string

const (
	// A NewOrder: inserts the order, PENDING, unless it is already stored.
	ChangeInsert ChangeKind = "insert"
	// An order's first event, OrderAccepted or OrderRejected, which may arrive
	// before its NewOrder: inserts the order with its status, or moves a
	// PENDING order to it. An order already out of PENDING is left as it is.
	ChangeFirstEvent ChangeKind = "first_event"
	// Cancels an order that is not final.
	ChangeCancel ChangeKind = "cancel"
	// The engine's quantity is what is still pending, so the order's quantity
	// becomes what it already executed plus that. A final order is left as it is.
	ChangeModify ChangeKind = "modify"
	// Records the trade and adds it to its two orders: a limit order is
	// PARTIALLY_FILLED until its quantity is executed, and a market order stays
	// PENDING until it is FILLED or its remainder is cancelled. A trade already
	// recorded changes nothing.
	ChangeTrade ChangeKind = "trade"
)

// Order is set for insert and first_event, Trade for trade; cancel and modify
// set OrderID and At, with Reason for cancel and Limit and PendingQuantity for
// modify.
type Change struct {
	Kind            ChangeKind
	Order           *models.Order
	Trade           *models.Trade
	OrderID         uuid.UUID
	Reason          *string
	Limit           int64
	PendingQuantity int64
	At              time.Time
}

type orderDB interface {
	applyChanges(ctx *gofr.Context, changes []Change) error
	listOrders(ctx *gofr.Context, query ListQuery) ([]models.Order, error)
	getOrder(ctx *gofr.Context, userID string, id uuid.UUID) (models.Order, error)
}

type postgres struct{}

var db orderDB = postgres{}

// Applies the changes in order, all of them or none. Each one is the statement
// of its kind, whose condition is the change it may make, so a repeated
// message, or a command and an event read in either order, never overwrite
// each other.
func ApplyChanges(ctx *gofr.Context, changes []Change) error {
	return db.applyChanges(ctx, changes)
}

func ListOrders(ctx *gofr.Context, query ListQuery) ([]models.Order, error) {
	return db.listOrders(ctx, query)
}

func GetOrder(ctx *gofr.Context, userID string, id uuid.UUID) (models.Order, error) {
	return db.getOrder(ctx, userID, id)
}
