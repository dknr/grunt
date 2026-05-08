package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"grunt/server/storage"
)

// apiImpl implements ServerInterface.
type apiImpl struct {
	store *storage.Store
	hub   *Hub
}

// RegisterUser implements the register endpoint.
func (a *apiImpl) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Registration attempt", "reason", "invalid request body")
		http.Error(w, `{"error":"missing user or password field"}`, http.StatusBadRequest)
		return
	}
	if req.User == "" || req.Password == "" {
		slog.Warn("Registration attempt", "user", req.User, "reason", "missing fields")
		http.Error(w, `{"error":"user and password are required"}`, http.StatusBadRequest)
		return
	}

	// Validate invite code (required for all registrations)
	valid, err := a.store.ValidateInvite(req.InviteCode)
	if err != nil {
		slog.Error("Error validating invite", "error", err)
		http.Error(w, `{"error":"failed to validate invite code"}`, http.StatusInternalServerError)
		return
	}
	if !valid {
		slog.Warn("Registration attempt with invalid invite", "user", req.User)
		http.Error(w, `{"error":"invalid or expired invite code"}`, http.StatusUnauthorized)
		return
	}

	if err := a.store.CreateUser(req.User, req.Password); err != nil {
		// Check if user already exists (409 Conflict)
		slog.Warn("User already registered", "user", req.User)
		w.WriteHeader(http.StatusConflict)
		msg := MessageResponse{
			Message: strPtr("user already exists"),
		}
		json.NewEncoder(w).Encode(msg)
		return
	}

	// Mark invite as used
	if err := a.store.MarkInviteUsed(req.InviteCode, req.User); err != nil {
		slog.Error("Error marking invite as used", "error", err)
		// Don't fail registration if invite marking fails
	}

	slog.Info("User registered", "user", req.User)
	w.WriteHeader(http.StatusCreated)
	msg := MessageResponse{
		Message: strPtr("user created"),
	}
	json.NewEncoder(w).Encode(msg)
}

// GetInvite implements the invite generation endpoint.
func (a *apiImpl) GetInvite(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by AuthMiddleware)
	userID := r.Context().Value(userIDKey).(string)

	// Generate new invite code
	code := generateInviteCode()
	expiresAt := time.Now().Add(10 * time.Minute)

	if err := a.store.CreateInvite(code, expiresAt, userID); err != nil {
		slog.Error("Error creating invite", "error", err)
		http.Error(w, `{"error":"failed to create invite code"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Invite code generated", "user", userID)
	resp := InviteResponse{
		Code:      strPtr(code),
		ExpiresAt: &expiresAt,
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// generateInviteCode generates a random invite code.
func generateInviteCode() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// LoginUser implements the login endpoint.
func (a *apiImpl) LoginUser(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"missing user or password field"}`, http.StatusBadRequest)
		return
	}
	ok, err := a.store.VerifyUser(req.User, req.Password)
	if err != nil {
		slog.Error("Error verifying user", "error", err)
		http.Error(w, `{"error":"failed to verify user"}`, http.StatusInternalServerError)
		return
	}
	if !ok {
		slog.Warn("Failed login attempt", "user", req.User)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	token, err := a.hub.GenerateToken(req.User)
	if err != nil {
		slog.Error("Error generating token", "error", err)
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}
	slog.Info("User logged in", "user", req.User)
	resp := LoginResponse{
		Token: strPtr(token),
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// SyncMessages implements the sync endpoint.
func (a *apiImpl) SyncMessages(w http.ResponseWriter, r *http.Request, params SyncMessagesParams) {
	since := int64(0)
	if params.Since != nil {
		since = *params.Since
	}
	last := int64(0)
	if params.Last != nil {
		last = *params.Last
	}

	msgs, err := a.store.Sync(int(since), int(last))
	if err != nil {
		slog.Error("Error syncing messages", "error", err)
		http.Error(w, `{"error":"failed to sync messages"}`, http.StatusInternalServerError)
		return
	}

	apiMsgs := make([]Broadcast, len(msgs))
	for i, m := range msgs {
		apiMsgs[i] = Broadcast{
			Type:      strPtr(m.Type),
			Id:        int64Ptr(int64(m.ID)),
			Content:   strPtr(m.Content),
			ClientId:  strPtr(m.ClientID),
			User:      strPtr(m.UserID),
			Timestamp: &m.Timestamp,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiMsgs)
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// int64Ptr returns a pointer to the given int64.
func int64Ptr(i int64) *int64 {
	return &i
}
