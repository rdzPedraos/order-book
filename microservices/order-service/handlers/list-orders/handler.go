// Package listorders serves GET /orders: the person's orders, newest first,
// filtered by status, side and book and paginated with a cursor.
package listorders

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/http/response"

	"github.com/rdzpedraos/order-book/microservices/order-service/models"
	"github.com/rdzpedraos/order-book/microservices/order-service/store/orderdb"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/identity"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

var errInvalidCursor = fault.New("invalid_cursor", "invalid cursor")

type request struct {
	UserID   string
	Status   *models.Status
	Side     *models.Side
	Book     *string
	Cursor   *uuid.UUID
	PageSize int
}

func Handle(ctx *gofr.Context) (any, error) {
	req, err := parseParams(ctx)
	if err != nil {
		return nil, err
	}

	page, err := req.listOrders(ctx)
	if err != nil {
		return nil, fault.From(err)
	}

	return *page, nil
}

// An empty parameter is not a filter.
func parseParams(ctx *gofr.Context) (*request, error) {
	userID, err := identity.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	book, err := parseBook(ctx.Param("book"))
	if err != nil {
		return nil, err
	}

	cursor, err := parseCursor(ctx.Param("cursor"))
	if err != nil {
		return nil, err
	}

	pageSize, err := parsePageSize(ctx.Param("limit"))
	if err != nil {
		return nil, err
	}

	status, err := parseStatus(ctx.Param("status"))
	if err != nil {
		return nil, err
	}

	side, err := parseSide(ctx.Param("side"))
	if err != nil {
		return nil, err
	}

	return &request{UserID: userID, Status: status, Side: side, Book: book, Cursor: cursor, PageSize: pageSize}, nil
}

// One extra row tells whether there is a next page without a count query.
func (r *request) listOrders(ctx *gofr.Context) (*response.Response, error) {
	orders, err := orderdb.ListOrders(ctx, orderdb.ListQuery{
		UserID: r.UserID,
		Status: r.Status,
		Side:   r.Side,
		Book:   r.Book,
		Cursor: r.Cursor,
		Limit:  r.PageSize + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	var nextCursor any

	if len(orders) > r.PageSize {
		orders = orders[:r.PageSize]
		nextCursor = orders[r.PageSize-1].ID.String()
	}

	if orders == nil {
		orders = []models.Order{}
	}

	return &response.Response{Data: orders, Metadata: map[string]any{"nextCursor": nextCursor}}, nil
}

func parseBook(value string) (*string, error) {
	if value == "" {
		return nil, nil
	}

	book, err := books.Normalize(value)
	if err != nil {
		return nil, err
	}

	return &book.ID, nil
}

func parseCursor(value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}

	cursor, err := uuid.Parse(value)
	if err != nil {
		return nil, errInvalidCursor
	}

	return &cursor, nil
}

func parsePageSize(value string) (int, error) {
	if value == "" {
		return defaultPageSize, nil
	}

	pageSize, err := strconv.Atoi(value)
	if err != nil || pageSize < 1 || pageSize > maxPageSize {
		return 0, models.ErrInvalidPageSize
	}

	return pageSize, nil
}

func parseStatus(value string) (*models.Status, error) {
	if value == "" {
		return nil, nil
	}

	status := models.Status(value)
	if !models.IsValidStatus(status) {
		return nil, models.ErrInvalidStatus
	}

	return &status, nil
}

func parseSide(value string) (*models.Side, error) {
	if value == "" {
		return nil, nil
	}

	side := models.Side(value)
	if !models.IsValidSide(side) {
		return nil, models.ErrInvalidSide
	}

	return &side, nil
}
