// Package cancelorder handles DELETE /orders/{id}. It validates the person and
// that the order is theirs, then answers 501 not_implemented: cancelling an
// order here would race with its execution, so cancellations arrive as
// commands in a later phase.
package cancelorder

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/identity"
)

type request struct {
	UserID  string
	OrderID uuid.UUID
}

func Handle(ctx *gofr.Context) (any, error) {
	req, err := parseRequest(ctx)
	if err != nil {
		return nil, err
	}

	if _, err := req.ownedOrder(ctx); err != nil {
		return nil, fault.From(err)
	}

	return nil, models.ErrNotImplemented
}

func parseRequest(ctx *gofr.Context) (*request, error) {
	userID, err := identity.UserID(ctx)
	if err != nil {
		return nil, err
	}

	orderID, err := uuid.Parse(ctx.PathParam("id"))
	if err != nil {
		return nil, models.ErrOrderNotFound
	}

	return &request{UserID: userID, OrderID: orderID}, nil
}

// The store filters by user_id, so someone else's order is not found.
func (r *request) ownedOrder(ctx *gofr.Context) (*models.Order, error) {
	order, err := orderdb.GetOrder(ctx, r.UserID, r.OrderID)
	if errors.Is(err, models.ErrOrderNotFound) {
		return nil, err
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	return &order, nil
}
