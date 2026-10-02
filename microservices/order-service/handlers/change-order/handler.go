// Package changeorder handles POST /orders/{id}/change: it validates the person,
// that the order is theirs and a limit order, and the body, then publishes a
// ModifyOrder and answers the order as it is. The engine decides whether the
// change is applied, and the order changes when its events are projected.
package changeorder

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/identity"
	"github.com/rdzpedraos/order-book/shared/money"
)

type request struct {
	UserID   string    `json:"-"`
	OrderID  uuid.UUID `json:"-"`
	Limit    *string   `json:"limit"`
	Quantity *string   `json:"quantity"`
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

	if order.Type != models.TypeLimit {
		return nil, models.ErrOrderNotModifiable
	}

	// The order gives the book, whose currencies parse the body amounts.
	book, err := books.Normalize(order.Book)
	if err != nil {
		return nil, err
	}

	modifyOrder, err := req.buildModifyOrder(book)
	if err != nil {
		return nil, err
	}

	err = publishModifyOrder(ctx, book.ID, modifyOrder)
	if err != nil {
		return nil, fault.From(err)
	}

	return order, nil
}

func parseRequest(ctx *gofr.Context) (*request, error) {
	var req request

	err := ctx.Bind(&req)
	if err != nil {
		return nil, fault.ErrInvalidBody
	}

	req.UserID, err = identity.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	req.OrderID, err = uuid.Parse(ctx.PathParam("id"))
	if err != nil {
		return nil, models.ErrOrderNotFound
	}

	return &req, nil
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

func (r *request) buildModifyOrder(book books.Book) (*events.ModifyOrder, error) {
	if r.Limit == nil && r.Quantity == nil {
		return nil, models.ErrNothingToModify
	}

	limit, err := parsePositiveAmount(book.Quote, r.Limit, models.ErrInvalidLimit)
	if err != nil {
		return nil, err
	}

	quantity, err := parsePositiveAmount(book.Base, r.Quantity, models.ErrInvalidQuantity)
	if err != nil {
		return nil, err
	}

	return &events.ModifyOrder{OrderID: r.OrderID, UserID: r.UserID, Limit: limit, Quantity: quantity}, nil
}

// A field that was not sent stays nil; a sent one must be a positive amount of its currency.
func parsePositiveAmount(currency money.Currency, value *string, invalid error) (*int64, error) {
	amount, err := money.Parse(currency, value)
	if err != nil || (amount != nil && *amount <= 0) {
		return nil, invalid
	}

	return amount, nil
}

func publishModifyOrder(ctx *gofr.Context, book string, modifyOrder *events.ModifyOrder) error {
	message, err := events.NewMessage(events.RouteModifyOrder, book, time.Now(), modifyOrder)
	if err != nil {
		return err
	}

	if err := producer.Publish(ctx, message); err != nil {
		return fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	return nil
}
