// Package identity identifies the person behind each request through the
// X-User-ID header, the convention every service applies the same way.
//
// It only depends on net/http and context, so any framework can register it
// (Gofr: app.UseMiddleware(identity.Middleware)).
package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/rdzpedraos/order-book/shared/fault"
)

// Trusted as is: a future gateway with real authentication will set it.
const Header = "X-User-ID"

// Gofr's health routes have no user.
var routesWithoutUser = map[string]bool{
	"/.well-known/alive":  true,
	"/.well-known/health": true,
}

var ErrMissingUserID = fault.NewWithStatus(http.StatusUnauthorized, "missing_user_id", "missing X-User-ID header")

type contextKey struct{}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if routesWithoutUser[r.URL.Path] {
			next.ServeHTTP(w, r)

			return
		}

		userID := strings.TrimSpace(r.Header.Get(Header))
		if userID == "" {
			ErrMissingUserID.Write(w)

			return
		}

		ctx := context.WithValue(r.Context(), contextKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(contextKey{}).(string)
	if !ok || userID == "" {
		return "", ErrMissingUserID
	}

	return userID, nil
}
