package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/auth"
	"github.com/var-raphael/groundtruth/internal/db/queries"
)

func isPublicPath(path string) bool {
	return path == "/health" ||
		path == "/ping" ||
		path == "/jobs/public" ||
		strings.HasPrefix(path, "/public/") ||
		strings.HasPrefix(path, "/shared/") ||
		strings.HasPrefix(path, "/webhooks/")
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func RecruiterAuth(verifier *auth.Verifier, pool *pgxpool.Pool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if header == "" || token == "" || token == header {
			writeAuthError(w, http.StatusUnauthorized, "sign in required")
			return
		}

		claims, err := verifier.Verify(r.Context(), token)
		if err != nil {
			log.Printf("recruiter auth: %v", err)
			writeAuthError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		if claims.Provider != "google" || claims.Email == "" {
			writeAuthError(w, http.StatusForbidden, "recruiter sign-in required")
			return
		}

		recruiter, err := queries.FindOrCreateRecruiterFromAuth(r.Context(), pool, claims.Sub, claims.Email, claims.Name)
		if err != nil {
			log.Printf("recruiter auth: %v", err)
			writeAuthError(w, http.StatusInternalServerError, "could not load your account")
			return
		}

		ctx := context.WithValue(r.Context(), RecruiterIDKey, recruiter.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
