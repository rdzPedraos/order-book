// Package modifyorder handles PATCH /orders/{id}. It validates the person, that
// the order is theirs and the body, then answers 501 not_implemented: changing
// an order here would race with its execution, so modifications arrive as
// commands in a later phase.
package modifyorder

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/books"
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

	order, err := req.ownedOrder(ctx)
	if err != nil {
		return nil, fault.From(err)
	}

	// The order gives the book, whose currencies parse the body amounts.
	book, err := books.Normalize(order.Book)
	if err != nil {
		return nil, err
	}

	if err := req.validate(book); err != nil {
		return nil, err
	}

	return nil, models.ErrNotImplemented
}

func parseRequest(ctx *gofr.Context) (*request, error) {
	var req request

	err := ctx.Bind(&req)
	if err != nil {
		return nil, fault.ErrInvalidBody
	}

	req.UserID, err = identity.UserID(ctx)
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

func (r *request) validate(book books.Book) error {
	if r.Limit == nil && r.Quantity == nil {
		return models.ErrNothingToModify
	}

	if !isPositive(book.Quote, r.Limit) {
		return models.ErrInvalidLimit
	}

	if !isPositive(book.Base, r.Quantity) {
		return models.ErrInvalidQuantity
	}

	return nil
}

// A field that was not sent is fine; a sent one must be a positive amount of its currency.
func isPositive(c money.Currency, value *string) bool {
	amount, err := money.Parse(c, value)

	return err == nil && (amount == nil || *amount > 0)
}
