package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

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
	if err := a.store.CreateUser(req.User, req.Password); err != nil {
		slog.Error("Error creating user", "error", err)
		http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		return
	}
	slog.Info("User registered", "user", req.User)
	w.WriteHeader(http.StatusCreated)
	msg := MessageResponse{
		Message: strPtr("user created"),
	}
	json.NewEncoder(w).Encode(msg)
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
