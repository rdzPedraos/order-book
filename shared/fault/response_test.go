package fault

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestFrom(t *testing.T) {
	notFound := NewWithStatus(http.StatusNotFound, "order_not_found", "order not found")

	t.Run("api error", func(t *testing.T) {
		c := require.New(t)

		c.Equal(notFound, From(notFound))
	})

	t.Run("wrapped api error", func(t *testing.T) {
		c := require.New(t)

		c.Equal(notFound, From(fmt.Errorf("get order: %w", notFound)))
	})

	t.Run("no error", func(t *testing.T) {
		c := require.New(t)

		c.NoError(From(nil))
	})

	t.Run("unknown error hides its message", func(t *testing.T) {
		c := require.New(t)

		c.Equal(ErrInternal, From(errors.New("dial tcp 10.0.0.5:5432: connection refused")))
	})
}
