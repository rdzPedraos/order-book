// Package createorder handles POST /orders: it parses the body, applies the
// order rules and publishes the order as a NewOrder command to the log.
package createorder

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/eventlog/events"
	"github.com/rdzpedraos/order-book/shared/eventlog/producer"
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
	userID, err := identity.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	var req createOrderRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, fault.ErrInvalidBody
	}

	order, err := buildOrder(userID, &req)
	if err != nil {
		return nil, err
	}

	if err := publishNewOrder(ctx, order); err != nil {
		return nil, fault.From(err)
	}

	return order, nil
}

// The order exists once its NewOrder is in the log; the insert-new-order
// subscriber stores it in orders afterwards.
func publishNewOrder(ctx *gofr.Context, order models.Order) error {
	message, err := events.NewMessage(events.RouteNewOrder, order.Book, order.CreatedAt, events.NewOrder{
		OrderID:  order.ID,
		UserID:   order.UserID,
		Side:     string(order.Side),
		Type:     string(order.Type),
		Limit:    order.Limit,
		Amount:   order.Amount,
		Quantity: order.Quantity,
	})
	if err != nil {
		return err
	}

	if err := producer.Publish(ctx, message); err != nil {
		return fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	return nil
}

func buildOrder(userID string, req *createOrderRequest) (models.Order, error) {
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

	orderID, err := uuid.NewV7()
	if err != nil {
		return models.Order{}, fmt.Errorf("generate order id: %w", err)
	}

	createdAt := time.Now().UTC()

	order := models.Order{
		ID:        orderID,
		UserID:    userID,
		Book:      book.ID,
		Side:      req.Side,
		Type:      getOrderType(req),
		Limit:     limit,
		Amount:    amount,
		Quantity:  quantity,
		Status:    models.StatusPending,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}

	if err := order.Validate(); err != nil {
		return models.Order{}, err
	}

	return order, nil
}

func getOrderType(req *createOrderRequest) models.OrderType {
	if req.Limit != nil {
		return models.TypeLimit
	}

	return models.TypeMarket
}
