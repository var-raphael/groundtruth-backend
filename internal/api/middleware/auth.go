package middleware

import (
	"context"
	"net/http"
)

const DevRecruiterID = "00000000-0000-0000-0000-000000000001"

func FakeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), RecruiterIDKey, DevRecruiterID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
