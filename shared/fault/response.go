package fault

import (
	"encoding/json"
	"net/http"

	"errors"
)

// Gofr answers with StatusCode() and merges Response() into {"error": {"message": ...}}.
func (e Error) StatusCode() int { return e.Status }

func (e Error) Response() map[string]any {
	return map[string]any{"code": e.Code}
}

// Gofr only renders an Error returned as is, so a wrapped one is unwrapped here,
// and any other error becomes ErrInternal so its message never reaches the client.
func From(err error) error {
	if err == nil {
		return nil
	}

	var appErr Error
	if errors.As(err, &appErr) {
		return appErr
	}

	return ErrInternal
}

func (e Error) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)

	body := map[string]any{
		"error": map[string]string{"code": e.Code, "message": e.Message},
	}

	_ = json.NewEncoder(w).Encode(body)
}
