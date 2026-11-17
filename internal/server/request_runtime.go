package server

import (
	"context"
	"net/http"
	"time"

	"portinaia/internal/logging"
)

// requestIdentityMiddleware establishes the single correlation identity used by
// problems, logs, and downstream audit calls. Mount before handlers/authentication.
func requestIdentityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			id, _ = randomID()
			id = "req_" + id
		}
		ctx := logging.WithRequestID(r.Context(), id)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func validRequestID(id string) bool {
	return logging.ValidRequestID(id)
}

func requestIdentity(ctx context.Context) string {
	return logging.RequestID(ctx)
}

// boundedExecutionMiddleware provides a per-request deadline. A caller must
// mount it before handlers and ensure every database/provider operation uses r.Context().
func boundedExecutionMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	return http.TimeoutHandler(next, timeout, "request deadline exceeded")
}
