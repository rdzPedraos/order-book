// Package fault declares every error of the system with the status and code it
// is answered with over HTTP (the format of docs/api.md). A consumer that is not
// HTTP, such as the matching engine, uses them as any other error.
//
// It is shared so that every service, and middlewares that run outside Gofr,
// answer an error with exactly the same body.
package fault

import (
	"net/http"
)

type Error struct {
	Status  int    // HTTP status: 400
	Code    string // stable identifier clients branch on, not a number: "invalid_quantity"
	Message string // human-readable text that may change: "invalid quantity"
}

var (
	ErrInvalidBody        = New("invalid_body", "request body is not valid JSON")
	ErrServiceUnavailable = NewWithStatus(http.StatusServiceUnavailable, "service_unavailable", "service temporarily unavailable")
	ErrInternal           = NewWithStatus(http.StatusInternalServerError, "internal_error", "internal error")
)

func New(code, message string) Error {
	return NewWithStatus(http.StatusBadRequest, code, message)
}

func NewWithStatus(status int, code, message string) Error {
	return Error{Status: status, Code: code, Message: message}
}

func (e Error) Error() string { return e.Message }
