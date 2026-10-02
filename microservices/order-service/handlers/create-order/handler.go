// Package createorder handles POST /orders: it parses the body, applies the
// order rules and stores the order as PENDING.
package createorder

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/identity"
	"github.com/rdzpedraos/order-book/shared/money"
)

type createOrderRequest struct {
	Book     string      `json:"book"`
	Side     models.Side `json:"side"`
	Limit    *string     `json:"limit"`
	Quantity *string     `json:"quantity"`
	Amount   *string     `json:"amount"`
}

func Handle(ctx *gofr.Context) (any, error) {
	userID, err := identity.UserID(ctx)
	if err != nil {
		return nil, err
	}

	var req createOrderRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, fault.ErrInvalidBody
	}

	order, err := newOrder(userID, &req)
	if err != nil {
		return nil, err
	}

	if err := orderdb.InsertOrder(ctx, order); err != nil {
		return nil, fault.From(fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err))
	}

	return order, nil
}

func newOrder(userID string, req *createOrderRequest) (models.Order, error) {
	book, err := books.Normalize(req.Book)
	if err != nil {
		return models.Order{}, err
	}

	limit, err := money.Parse(book.Quote, req.Limit)
	if err != nil {
		return models.Order{}, models.ErrInvalidLimit
	}

	amount, err := money.Parse(book.Quote, req.Amount)
	if err != nil {
		return models.Order{}, models.ErrInvalidAmount
	}

	quantity, err := money.Parse(book.Base, req.Quantity)
	if err != nil {
		return models.Order{}, models.ErrInvalidQuantity
	}

	id, err := uuid.NewV7()
	if err != nil {
		return models.Order{}, fmt.Errorf("generate order id: %w", err)
	}

	now := time.Now().UTC()

	order := models.Order{
		ID:        id,
		UserID:    userID,
		Book:      book.ID,
		Side:      req.Side,
		Type:      orderType(req),
		Limit:     limit,
		Amount:    amount,
		Quantity:  quantity,
		Status:    models.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := order.Validate(); err != nil {
		return models.Order{}, err
	}

	return order, nil
}

func orderType(req *createOrderRequest) models.OrderType {
	if req.Limit != nil {
		return models.TypeLimit
	}

	return models.TypeMarket
}
