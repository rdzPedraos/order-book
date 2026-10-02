package models

import (
	"net/http"

	"github.com/rdzpedraos/order-book/shared/fault"
)

var (
	ErrInvalidDepth = fault.New("invalid_depth", "depth must be between 1 and 100")
	ErrBookNotFound = fault.NewWithStatus(http.StatusNotFound, "book_not_found", "book not found")
)
