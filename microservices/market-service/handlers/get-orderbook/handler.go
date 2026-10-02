// Package getorderbook handles GET /market/orderbook/{book}: it answers the
// public depth of the book, its best levels on each side, without orders or
// people. It needs no X-User-ID.
package getorderbook

import (
	"fmt"
	"strconv"

	"gofr.dev/pkg/gofr"

	"github.com/rdzpedraos/order-book/microservices/market-service/models"
	"github.com/rdzpedraos/order-book/microservices/market-service/store/marketdb"
	"github.com/rdzpedraos/order-book/shared/books"
	"github.com/rdzpedraos/order-book/shared/fault"
)

const (
	defaultDepth = 20
	maxDepth     = 100
)

type request struct {
	Book  string
	Depth int
}

func Handle(ctx *gofr.Context) (any, error) {
	req, err := parseRequest(ctx)
	if err != nil {
		return nil, err
	}

	orderBook, err := req.getOrderBook(ctx)
	if err != nil {
		return nil, fault.From(err)
	}

	return *orderBook, nil
}

// An unknown book is not found rather than invalid, like any resource of the path.
func parseRequest(ctx *gofr.Context) (*request, error) {
	book, err := books.Normalize(ctx.PathParam("book"))
	if err != nil {
		return nil, models.ErrBookNotFound
	}

	depth, err := parseDepth(ctx.Param("depth"))
	if err != nil {
		return nil, err
	}

	return &request{Book: book.ID, Depth: depth}, nil
}

func parseDepth(value string) (int, error) {
	if value == "" {
		return defaultDepth, nil
	}

	depth, err := strconv.Atoi(value)
	if err != nil || depth < 1 || depth > maxDepth {
		return 0, models.ErrInvalidDepth
	}

	return depth, nil
}

func (r *request) getOrderBook(ctx *gofr.Context) (*models.OrderBook, error) {
	bids, err := marketdb.ListLevels(ctx, r.Book, "BUY", r.Depth)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	asks, err := marketdb.ListLevels(ctx, r.Book, "SELL", r.Depth)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", fault.ErrServiceUnavailable, err)
	}

	return &models.OrderBook{Book: r.Book, Bids: bids, Asks: asks}, nil
}
