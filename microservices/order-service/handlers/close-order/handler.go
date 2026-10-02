// Package closeorder handles POST /orders/{id}/close: it validates the person
// and that the order is theirs, then publishes a CancelOrder and answers the
// order as it is. The engine decides whether the order is cancelled, and the
// order changes when its events are projected.
package closeorder

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
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

	order, err := req.getOwnedOrder(ctx)
	if err != nil {
		return nil, fault.From(err)
	}

	err = publishCancelOrder(ctx, order, req.UserID)
	if err != nil {
		return nil, fault.From(err)
	}

	return order, nil
}

func publishCancelOrder(ctx *gofr.Context, order *models.Order, userID string) error {
	message, err := events.NewMessage(events.RouteCancelOrder, order.Book, time.Now(), events.CancelOrder{OrderID: order.ID, UserID: userID})
	if err != nil {
		return err
	}

	if err := producer.Publish(ctx, message); err != nil {
		return fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	return nil
}

func parseRequest(ctx *gofr.Context) (*request, error) {
	userID, err := identity.GetUserID(ctx)
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
func (r *request) getOwnedOrder(ctx *gofr.Context) (*models.Order, error) {
	order, err := orderdb.GetOrder(ctx, r.UserID, r.OrderID)
	if errors.Is(err, models.ErrOrderNotFound) {
		return nil, err
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	return &order, nil
}
