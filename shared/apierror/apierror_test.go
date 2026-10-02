package apierror

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("defaults to bad request", func(t *testing.T) {
		c := require.New(t)

		err := New("invalid_quantity", "invalid quantity")

		c.Equal(http.StatusBadRequest, err.Status)
		c.Equal("invalid_quantity", err.Code)
		c.Equal("invalid quantity", err.Message)
	})

	t.Run("with an explicit status", func(t *testing.T) {
		c := require.New(t)

		err := NewWithStatus(http.StatusNotFound, "order_not_found", "order not found")

		c.Equal(http.StatusNotFound, err.Status)
		c.Equal("order_not_found", err.Code)
		c.Equal("order not found", err.Message)
	})
}

func TestWrite(t *testing.T) {
	c := require.New(t)

	rec := httptest.NewRecorder()
	NewWithStatus(http.StatusUnauthorized, "missing_user_id", "missing X-User-ID header").Write(rec)

	c.Equal(http.StatusUnauthorized, rec.Code)
	c.Equal("application/json", rec.Header().Get("Content-Type"))
	c.JSONEq(`{"error": {"code": "missing_user_id", "message": "missing X-User-ID header"}}`, rec.Body.String())
}

func TestGofrRendering(t *testing.T) {
	err := NewWithStatus(http.StatusNotFound, "order_not_found", "order not found")

	t.Run("status code", func(t *testing.T) {
		c := require.New(t)

		c.Equal(http.StatusNotFound, err.StatusCode())
	})

	t.Run("code merged into the error object", func(t *testing.T) {
		c := require.New(t)

		c.Equal(map[string]any{"code": "order_not_found"}, err.Response())
	})

	t.Run("message", func(t *testing.T) {
		c := require.New(t)

		c.Equal("order not found", err.Error())
	})
}
