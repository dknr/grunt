package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"grunt/server/storage"
)

type contextKey string

const (
	userIDKey        contextKey = "userID"
	isAdminKey       contextKey = "isAdmin"
	authenticatedKey contextKey = "authenticated"
)

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

// ValidateToken checks if the given token is valid and returns the associated user ID.
func ValidateToken(token string, store *storage.Store) (string, bool) {
	// Check if it's an API key (starts with gk_)
	if strings.HasPrefix(token, "gk_") {
		keyHash := storage.HashAPIKey(token)
		userID, err := store.GetAPIKeyByHash(keyHash)
		if err != nil {
			return "", false
		}
		return userID, true
	}

	// Session token validation (existing logic from hub.go)
	tokenStore.mu.RLock()
	defer tokenStore.mu.RUnlock()
	entry, ok := tokenStore.tokens[token]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.Expiry) {
		delete(tokenStore.tokens, token)
		return "", false
	}
	return entry.UserID, true
}

// authMiddleware applies authentication to all requests.
// It populates the request context with userID, isAdmin, and authenticated status.
// Handlers should check AuthenticatedFromContext() to determine if they need to reject unauthenticated requests.
func authMiddleware(next http.Handler, store *storage.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ExtractToken(r)

		var userID string
		var isAdmin bool
		var authenticated bool

		if token != "" {
			userID, authenticated = ValidateToken(token, store)
			if authenticated {
				isAdmin, _ = store.IsUserAdmin(userID)
			} else {
				userID = ""
			}
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		ctx = context.WithValue(ctx, isAdminKey, isAdmin)
		ctx = context.WithValue(ctx, authenticatedKey, authenticated)

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

// IsAdminFromContext extracts the isAdmin status from the request context.
func IsAdminFromContext(r *http.Request) bool {
	if isAdmin, ok := r.Context().Value(isAdminKey).(bool); ok {
		return isAdmin
	}
	return false
}

// AuthenticatedFromContext extracts the authenticated status from the request context.
func AuthenticatedFromContext(r *http.Request) bool {
	if authenticated, ok := r.Context().Value(authenticatedKey).(bool); ok {
		return authenticated
	}
	return false
}
