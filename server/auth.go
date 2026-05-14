package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userIDKey contextKey = "userID"

// ExtractToken checks for a token in the Authorization header, Cookie, or Query parameter.
// It returns the token string or an empty string if not found.
func ExtractToken(r *http.Request) string {
	// 1. Authorization Header
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != "" {
		return token
	}

	// 2. Cookie
	cookie, err := r.Cookie("token")
	if err == nil {
		return cookie.Value
	}

	// 3. Query Parameter (Legacy/HTMX SSE)
	return r.URL.Query().Get("token")
}

// authMiddleware applies authentication to requests that require it.
// Public endpoints are exempt.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Public endpoints that don't require auth
		switch {
		case r.Method == http.MethodPost && (r.URL.Path == "/api/user" || r.URL.Path == "/api/user/login"):
			next.ServeHTTP(w, r)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/":
			next.ServeHTTP(w, r)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/login":
			next.ServeHTTP(w, r)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			next.ServeHTTP(w, r)
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/static/"):
			next.ServeHTTP(w, r)
			return
		}

		// Authenticate using the unified ExtractToken function
		token := ExtractToken(r)
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
