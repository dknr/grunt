package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"grunt/server/storage"
)

func TestExtractToken_AuthorizationHeader(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")
	token := ExtractToken(req)
	if token != "test-token-123" {
		t.Errorf("Expected test-token-123, got %q", token)
	}
}

func TestExtractToken_AuthorizationHeader_NoBearer(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "test-token")
	token := ExtractToken(req)
	if token != "" {
		t.Errorf("Expected empty token for non-Bearer header, got %q", token)
	}
}

func TestExtractToken_Cookie(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "cookie-token"})
	token := ExtractToken(req)
	if token != "cookie-token" {
		t.Errorf("Expected cookie-token, got %q", token)
	}
}

func TestExtractToken_QueryParam(t *testing.T) {
	req, _ := http.NewRequest("GET", "/?token=query-token", nil)
	token := ExtractToken(req)
	if token != "query-token" {
		t.Errorf("Expected query-token, got %q", token)
	}
}

func TestExtractToken_QueryParamOverridesHeader(t *testing.T) {
	req, _ := http.NewRequest("GET", "/?token=query-token", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	// Authorization header takes priority
	token := ExtractToken(req)
	if token != "header-token" {
		t.Errorf("Expected header-token (header priority), got %q", token)
	}
}

func TestExtractToken_NoAuth(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	token := ExtractToken(req)
	if token != "" {
		t.Errorf("Expected empty token, got %q", token)
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	userID, ok := ValidateToken("", store)
	if ok {
		t.Error("Expected empty token to be invalid")
	}
	if userID != "" {
		t.Errorf("Expected empty userID, got %q", userID)
	}
}

func TestValidateToken_InvalidSessionToken(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	userID, ok := ValidateToken("nonexistent-token", store)
	if ok {
		t.Error("Expected nonexistent token to be invalid")
	}
	if userID != "" {
		t.Errorf("Expected empty userID, got %q", userID)
	}
}

func TestValidateToken_ValidSessionToken(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Generate a token via hub
	hub := NewHub(store)
	token, err := hub.GenerateToken("alice")
	if err != nil {
		t.Fatal(err)
	}

	userID, ok := ValidateToken(token, store)
	if !ok {
		t.Error("Expected valid token to be valid")
	}
	if userID != "alice" {
		t.Errorf("Expected alice, got %q", userID)
	}
}

func TestValidateToken_ExpiredSessionToken(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Manually insert an expired token
	tokenStore.mu.Lock()
	tokenStore.tokens["expired-token"] = &TokenEntry{
		UserID: "bob",
		Expiry: time.Now().Add(-1 * time.Hour), // 1 hour ago
	}
	tokenStore.mu.Unlock()

	userID, ok := ValidateToken("expired-token", store)
	if ok {
		t.Error("Expected expired token to be invalid")
	}
	if userID != "" {
		t.Errorf("Expected empty userID, got %q", userID)
	}

	// Verify token was deleted from store
	tokenStore.mu.Lock()
	_, exists := tokenStore.tokens["expired-token"]
	tokenStore.mu.Unlock()
	if exists {
		t.Error("Expected expired token to be deleted from store")
	}
}

func TestValidateToken_ValidAPIKey(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a user and API key
	if err := store.CreateUserNoPassword("bot"); err != nil {
		t.Fatal(err)
	}

	rawKey := "gk_" + strings.Repeat("aa", 16) // 32 hex chars
	keyHash := storage.HashAPIKey(rawKey)
	if _, err := store.CreateAPIKey("bot", "admin", keyHash, "test-key"); err != nil {
		t.Fatal(err)
	}

	userID, ok := ValidateToken(rawKey, store)
	if !ok {
		t.Error("Expected valid API key to be valid")
	}
	if userID != "bot" {
		t.Errorf("Expected bot, got %q", userID)
	}
}

func TestValidateToken_InvalidAPIKey(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	userID, ok := ValidateToken("gk_nonexistentkey000000000000000000", store)
	if ok {
		t.Error("Expected invalid API key to be invalid")
	}
	if userID != "" {
		t.Errorf("Expected empty userID, got %q", userID)
	}
}

func TestValidateToken_ExpiryBoundary(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Token expiring exactly now (should be valid if not yet expired)
	tokenStore.mu.Lock()
	tokenStore.tokens["boundary-token"] = &TokenEntry{
		UserID: "carol",
		Expiry: time.Now().Add(1 * time.Second), // expires in 1 second
	}
	tokenStore.mu.Unlock()

	userID, ok := ValidateToken("boundary-token", store)
	if !ok {
		t.Error("Expected token about to expire to still be valid")
	}
	if userID != "carol" {
		t.Errorf("Expected carol, got %q", userID)
	}
}

func TestUserIDFromContext_Present(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, userIDKey, "alice")
	req = req.WithContext(ctx)

	userID := UserIDFromContext(req)
	if userID != "alice" {
		t.Errorf("Expected alice, got %q", userID)
	}
}

func TestUserIDFromContext_Missing(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	userID := UserIDFromContext(req)
	if userID != "" {
		t.Errorf("Expected empty, got %q", userID)
	}
}

func TestIsAdminFromContext_True(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, isAdminKey, true)
	req = req.WithContext(ctx)

	if !IsAdminFromContext(req) {
		t.Error("Expected IsAdminFromContext to be true")
	}
}

func TestIsAdminFromContext_False(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, isAdminKey, false)
	req = req.WithContext(ctx)

	if IsAdminFromContext(req) {
		t.Error("Expected IsAdminFromContext to be false")
	}
}

func TestIsAdminFromContext_Missing(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	if IsAdminFromContext(req) {
		t.Error("Expected IsAdminFromContext to be false when missing")
	}
}

func TestAuthenticatedFromContext_True(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, authenticatedKey, true)
	req = req.WithContext(ctx)

	if !AuthenticatedFromContext(req) {
		t.Error("Expected AuthenticatedFromContext to be true")
	}
}

func TestAuthenticatedFromContext_False(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, authenticatedKey, false)
	req = req.WithContext(ctx)

	if AuthenticatedFromContext(req) {
		t.Error("Expected AuthenticatedFromContext to be false")
	}
}

func TestAuthenticatedFromContext_Missing(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	if AuthenticatedFromContext(req) {
		t.Error("Expected AuthenticatedFromContext to be false when missing")
	}
}

func TestValidateToken_ConcurrentSafe(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hub := NewHub(store)

	// Generate tokens under concurrent load
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				token, err := hub.GenerateToken("alice")
				if err != nil {
					t.Errorf("GenerateToken error: %v", err)
					return
				}
				userID, ok := ValidateToken(token, store)
				if !ok || userID != "alice" {
					t.Errorf("ValidateToken failed for generated token")
					return
				}
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestValidateToken_ConcurrentExpiry(t *testing.T) {
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert an expired token that multiple goroutines will race to delete
	tokenStore.mu.Lock()
	tokenStore.tokens["concurrent-expired"] = &TokenEntry{
		UserID: "alice",
		Expiry: time.Now().Add(-1 * time.Minute),
	}
	tokenStore.mu.Unlock()

	done := make(chan struct{}, 20)
	for i := 0; i < 20; i++ {
		go func() {
			ValidateToken("concurrent-expired", store)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify token was deleted exactly once (no panic, map intact)
	tokenStore.mu.Lock()
	_, exists := tokenStore.tokens["concurrent-expired"]
	tokenStore.mu.Unlock()
	if exists {
		t.Error("Expected expired token to be deleted")
	}

	// Verify store is still usable
	h := NewHub(store)
	_, err = h.GenerateToken("bob")
	if err != nil {
		t.Error("Store should still be usable after concurrent expiry")
	}
}
