package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userIDKey contextKey = "userID"

// authMiddleware applies authentication to requests that require it.
// Public endpoints (login, register) are exempt.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Public endpoints that don't require auth
		if r.Method == http.MethodPost && (r.URL.Path == "/api/user" || r.URL.Path == "/api/user/login") {
			next.ServeHTTP(w, r)
			return
		}

		// Authenticate: try Authorization header first, fall back to query param
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
			return
		}

		userID, ok := ValidateToken(token)
		if !ok {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext extracts the userID from the request context.
func UserIDFromContext(r *http.Request) string {
	if userID, ok := r.Context().Value(userIDKey).(string); ok {
		return userID
	}
	return ""
}
