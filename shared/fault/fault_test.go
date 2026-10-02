package fault

import (
	"net/http"
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

func TestCommonErrors(t *testing.T) {
	t.Run("invalid body", func(t *testing.T) {
		c := require.New(t)

		c.Equal(New("invalid_body", "request body is not valid JSON"), ErrInvalidBody)
	})

	t.Run("service unavailable", func(t *testing.T) {
		c := require.New(t)

		c.Equal(http.StatusServiceUnavailable, ErrServiceUnavailable.Status)
		c.Equal("service_unavailable", ErrServiceUnavailable.Code)
	})

	t.Run("internal error", func(t *testing.T) {
		c := require.New(t)

		c.Equal(NewWithStatus(http.StatusInternalServerError, "internal_error", "internal error"), ErrInternal)
	})
}
