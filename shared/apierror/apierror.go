// Package apierror is the error format of docs/api.md:
// {"error": {"code": "...", "message": "..."}} with its HTTP status.
//
// It is shared so that every service, and middlewares that run outside Gofr,
// answer errors with exactly the same body.
package apierror

import (
	"encoding/json"
	"net/http"
)

type Error struct {
	Status  int    // HTTP status: 400
	Code    string // stable identifier clients branch on, not a number: "invalid_quantity"
	Message string // human-readable text that may change: "invalid quantity"
}

func New(code, message string) Error {
	return NewWithStatus(http.StatusBadRequest, code, message)
}

func NewWithStatus(status int, code, message string) Error {
	return Error{Status: status, Code: code, Message: message}
}

func (e Error) Error() string { return e.Message }

// StatusCode and Response are what Gofr reads to render an error: it answers with
// StatusCode() and merges Response() into {"error": {"message": ...}}.
func (e Error) StatusCode() int { return e.Status }

func (e Error) Response() map[string]any {
	return map[string]any{"code": e.Code}
}

// For plain net/http middlewares, which run outside Gofr's responder.
func (e Error) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)

	body := map[string]any{
		"error": map[string]string{"code": e.Code, "message": e.Message},
	}

	_ = json.NewEncoder(w).Encode(body)
}
