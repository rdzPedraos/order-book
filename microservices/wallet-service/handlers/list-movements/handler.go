// Package listmovements handles GET /wallet/movements: the movements of the
// person's wallet that moved money, newest first, filtered by a date range
// and paginated with a cursor.
package listmovements

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"
	"gofr.dev/pkg/gofr/http/response"

	"github.com/rdzpedraos/order-book/microservices/wallet-service/models"
	"github.com/rdzpedraos/order-book/microservices/wallet-service/store/walletdb"
	"github.com/rdzpedraos/order-book/shared/fault"
	"github.com/rdzpedraos/order-book/shared/identity"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

var (
	errInvalidDate     = fault.New("invalid_date", "from and to must be RFC 3339 timestamps")
	errInvalidCursor   = fault.New("invalid_cursor", "invalid cursor")
	errInvalidPageSize = fault.New("invalid_limit", "limit must be between 1 and 100")
)

type request struct {
	UserID   string
	From     *time.Time
	To       *time.Time
	Cursor   *uuid.UUID
	PageSize int
}

func Handle(ctx *gofr.Context) (any, error) {
	req, err := parseParams(ctx)
	if err != nil {
		return nil, err
	}

	page, err := req.listMovements(ctx)
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

	from, err := parseDate(ctx.Param("from"))
	if err != nil {
		return nil, err
	}

	to, err := parseDate(ctx.Param("to"))
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

	return &request{UserID: userID, From: from, To: to, Cursor: cursor, PageSize: pageSize}, nil
}

// One extra row tells whether there is a next page without a count query.
func (r *request) listMovements(ctx *gofr.Context) (*response.Response, error) {
	movements, err := walletdb.ListMovements(ctx, walletdb.MovementQuery{
		UserID: r.UserID, From: r.From, To: r.To, Cursor: r.Cursor, Limit: r.PageSize + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	var nextCursor any

	if len(movements) > r.PageSize {
		movements = movements[:r.PageSize]
		nextCursor = movements[r.PageSize-1].ID.String()
	}

	if movements == nil {
		movements = []models.Movement{}
	}

	return &response.Response{Data: movements, Metadata: map[string]any{"nextCursor": nextCursor}}, nil
}

func parseDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}

	date, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, errInvalidDate
	}

	return &date, nil
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
		return 0, errInvalidPageSize
	}

	return pageSize, nil
}
