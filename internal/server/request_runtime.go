package server

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type requestIdentityKey struct{}

// requestIdentityMiddleware establishes the single correlation identity used by
// problems, logs, and downstream audit calls. Mount before handlers/authentication.
func requestIdentityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			id, _ = randomID()
			id = "req_" + id
		}
		ctx := context.WithValue(r.Context(), requestIdentityKey{}, id)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}

func requestIdentity(ctx context.Context) string {
	id, _ := ctx.Value(requestIdentityKey{}).(string)
	return id
}

// boundedExecutionMiddleware provides a per-request deadline. A caller must
// mount it before handlers and ensure every database/provider operation uses r.Context().
func boundedExecutionMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	return http.TimeoutHandler(next, timeout, "request deadline exceeded")
}
