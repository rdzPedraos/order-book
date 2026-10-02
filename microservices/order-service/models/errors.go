package models

import (
	"net/http"

	"github.com/rdzpedraos/order-book/shared/fault"
)

var (
	ErrInvalidSide      = fault.New("invalid_side", "side must be BUY or SELL")
	ErrInvalidQuantity  = fault.New("invalid_quantity", "invalid quantity")
	ErrInvalidLimit     = fault.New("invalid_limit_price", "invalid limit price")
	ErrInvalidAmount    = fault.New("invalid_amount", "invalid amount")
	ErrInvalidPageSize  = fault.New("invalid_limit", "limit must be between 1 and 100")
	ErrInvalidStatus    = fault.New("invalid_status", "invalid status")
	ErrNothingToModify  = fault.New("invalid_body", "limit or quantity is required")
	ErrUnsupportedOrder = fault.New("unsupported_order", "unsupported combination of limit, quantity and amount")
)

var (
	ErrOrderNotFound  = fault.NewWithStatus(http.StatusNotFound, "order_not_found", "order not found")
	ErrNotImplemented = fault.NewWithStatus(http.StatusNotImplemented, "not_implemented", "not implemented yet")
)
