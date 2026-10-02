package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func serveMiddleware(path, userIDHeader string) (*http.Request, *httptest.ResponseRecorder) {
	var reachedNext *http.Request
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { reachedNext = r })

	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	if userIDHeader != "" {
		req.Header.Set(Header, userIDHeader)
	}

	rec := httptest.NewRecorder()
	Middleware(next).ServeHTTP(rec, req)

	return reachedNext, rec
}

func TestMiddleware(t *testing.T) {
	t.Run("missing header", func(t *testing.T) {
		c := require.New(t)

		reachedNext, rec := serveMiddleware("/orders", "")
		c.Nil(reachedNext)
		c.Equal(http.StatusUnauthorized, rec.Code)
		c.JSONEq(`{"error": {"code": "missing_user_id", "message": "missing X-User-ID header"}}`, rec.Body.String())
	})

	t.Run("header present", func(t *testing.T) {
		c := require.New(t)

		reachedNext, _ := serveMiddleware("/orders", "user-a")
		c.NotNil(reachedNext)

		userID, err := GetUserID(reachedNext.Context())
		c.NoError(err)
		c.Equal("user-a", userID)
	})

	t.Run("header with surrounding spaces", func(t *testing.T) {
		c := require.New(t)

		reachedNext, _ := serveMiddleware("/orders", "  user-a  ")
		c.NotNil(reachedNext)

		userID, err := GetUserID(reachedNext.Context())
		c.NoError(err)
		c.Equal("user-a", userID)
	})

	t.Run("header with only spaces", func(t *testing.T) {
		c := require.New(t)

		reachedNext, rec := serveMiddleware("/orders", "   ")
		c.Nil(reachedNext)
		c.Equal(http.StatusUnauthorized, rec.Code)
	})
}

func TestMiddlewareOperationalRoutes(t *testing.T) {
	t.Run("health route needs no user", func(t *testing.T) {
		c := require.New(t)

		reachedNext, _ := serveMiddleware("/.well-known/health", "")
		c.NotNil(reachedNext)
	})

	t.Run("alive route needs no user", func(t *testing.T) {
		c := require.New(t)

		reachedNext, _ := serveMiddleware("/.well-known/alive", "")
		c.NotNil(reachedNext)
	})

	t.Run("other well-known route needs a user", func(t *testing.T) {
		c := require.New(t)

		reachedNext, rec := serveMiddleware("/.well-known/swagger", "")
		c.Nil(reachedNext)
		c.Equal(http.StatusUnauthorized, rec.Code)
	})
}

func TestUserID(t *testing.T) {
	t.Run("user id stored by the middleware", func(t *testing.T) {
		c := require.New(t)

		got, err := GetUserID(context.WithValue(context.Background(), contextKey{}, "user-a"))
		c.NoError(err)
		c.Equal("user-a", got)
	})

	t.Run("context without user id", func(t *testing.T) {
		c := require.New(t)

		_, err := GetUserID(context.Background())
		c.ErrorIs(err, ErrMissingUserID)
	})

	t.Run("empty user id", func(t *testing.T) {
		c := require.New(t)

		_, err := GetUserID(context.WithValue(context.Background(), contextKey{}, ""))
		c.ErrorIs(err, ErrMissingUserID)
	})

	t.Run("value that is not a string", func(t *testing.T) {
		c := require.New(t)

		_, err := GetUserID(context.WithValue(context.Background(), contextKey{}, 42))
		c.ErrorIs(err, ErrMissingUserID)
	})
}
