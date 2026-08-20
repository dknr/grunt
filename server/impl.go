package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"grunt/client"
	"grunt/server/storage"
)

// apiImpl implements ServerInterface.
type apiImpl struct {
	store *storage.Store
	hub   *Hub
}

// MaxMessageLength is the maximum allowed message content size in bytes.
const MaxMessageLength = 10240

// requireAdmin is a middleware that checks if the current user is an admin.
func (a *apiImpl) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r)
		if userID == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		isAdmin, err := a.store.IsUserAdmin(userID)
		if err != nil {
			slog.Error("Error checking admin status", "error", err)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		if !isAdmin {
			http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
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
	created, err := a.store.RegisterWithInvite(req.InviteCode, req.User, req.Password)
	if err != nil {
		// Check if it's a duplicate user error
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			slog.Warn("User already registered", "user", req.User)
			w.WriteHeader(http.StatusConflict)
			msg := MessageResponse{
				Message: strPtr("user already exists"),
			}
			json.NewEncoder(w).Encode(msg)
			return
		}
		slog.Warn("Registration failed", "user", req.User, "error", err)
		http.Error(w, `{"error":"`+html.EscapeString(err.Error())+`"}`, http.StatusUnauthorized)
		return
	}
	if !created {
		slog.Warn("Registration attempt with invalid invite", "user", req.User)
		http.Error(w, `{"error":"invalid or expired invite code"}`, http.StatusUnauthorized)
		return
	}

	// Set first user as admin
	userCount, err := a.store.CreateUserCount()
	if err == nil && userCount == 1 {
		if err := a.store.SetUserAdmin(req.User); err != nil {
			slog.Error("Failed to set first user as admin", "error", err)
		} else {
			slog.Info("First user registered, granted admin privileges", "user", req.User)
		}
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
	if !AuthenticatedFromContext(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Get user ID from context (set by AuthMiddleware)
	userID := UserIDFromContext(r)

	// Generate new invite code
	code := generateInviteCode()
	expiresAt := time.Now().Add(10 * time.Minute)

	if err := a.store.CreateInvite(code, expiresAt, userID); err != nil {
		slog.Error("Error creating invite", "error", err)
		if isHTMXRequest(r) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Failed to create invite code.</p>`))
			return
		}
		http.Error(w, `{"error":"failed to create invite code"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Invite code generated", "user", userID)

	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html")
		resultHTML := fmt.Sprintf(`<p class="success">New invite code: <strong>%s</strong> — expires %s</p>`,
			html.EscapeString(code), html.EscapeString(expiresAt.Format("2006-01-02 15:04")))
		w.Write([]byte(resultHTML))
		return
	}

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

// ChangePassword implements the password change endpoint.
func (a *apiImpl) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req ChangePasswordRequest
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"missing current_password or new_password field"}`, http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.CurrentPassword = r.FormValue("current_password")
		req.NewPassword = r.FormValue("new_password")
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		http.Error(w, `{"error":"current_password and new_password are required"}`, http.StatusBadRequest)
		return
	}

	userID := UserIDFromContext(r)
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if err := a.store.ChangePassword(userID, req.CurrentPassword, req.NewPassword); err != nil {
		// Check if it's an incorrect password error (401)
		if strings.Contains(err.Error(), "incorrect") {
			slog.Warn("Password change failed: wrong current password", "user", userID)
			http.Error(w, `{"error":"current password is incorrect"}`, http.StatusUnauthorized)
			return
		}
		slog.Error("Error changing password", "error", err)
		http.Error(w, `{"error":"failed to change password"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Password changed successfully", "user", userID)
	w.WriteHeader(http.StatusOK)
	msg := MessageResponse{
		Message: strPtr("password changed"),
	}
	json.NewEncoder(w).Encode(msg)
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

// SendMessage implements the send message endpoint.
func (a *apiImpl) SendMessage(w http.ResponseWriter, r *http.Request) {
	slog.Info("SendMessage handler called")

	// Early rejection based on Content-Length header
	if r.ContentLength > MaxMessageLength {
		slog.Warn("SendMessage rejected: Content-Length exceeds limit", "content_length", r.ContentLength)
		http.Error(w, `{"error":"content too large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	var req SendMessageRequest
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			slog.Warn("SendMessage decode error", "error", err)
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			slog.Warn("SendMessage parse form error", "error", err)
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.Content = r.FormValue("content")
	}
	if req.Content == "" {
		http.Error(w, `{"error":"content is required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Content) > MaxMessageLength {
		slog.Warn("SendMessage rejected: content exceeds length limit", "length", len(req.Content))
		http.Error(w, `{"error":"content exceeds maximum length"}`, http.StatusRequestEntityTooLarge)
		return
	}

	userID := UserIDFromContext(r)
	slog.Info("SendMessage userID from context", "userID", userID)
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	broadcast := &client.Broadcast{
		Type:      "message",
		Content:   req.Content,
		UserID:    userID,
		Timestamp: time.Now(),
	}

	id, err := a.store.Save(broadcast)
	if err != nil {
		slog.Error("Error saving message", "error", err)
		http.Error(w, `{"error":"failed to save message"}`, http.StatusInternalServerError)
		return
	}
	broadcast.ID = int(id)

	slog.Info("Message saved", "id", id, "content", req.Content)

	// Broadcast to all connected clients
	data, err := json.Marshal(broadcast)
	if err != nil {
		slog.Error("Error marshaling broadcast", "error", err)
		http.Error(w, `{"error":"failed to broadcast message"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Broadcasting message", "id", id, "content", req.Content)
	a.hub.BroadcastMessage(data)
	slog.Info("Message broadcast sent to hub", "id", id)

	resp := SendMessageResponse{
		Id: int64Ptr(id),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// StreamMessages delegates SSE stream handling to the Hub.
func (a *apiImpl) StreamMessages(w http.ResponseWriter, r *http.Request) {
	a.hub.HandleSSEStream(w, r)
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// int64Ptr returns a pointer to the given int64.
func int64Ptr(i int64) *int64 {
	return &i
}

// AdminCreateUser implements the admin create user endpoint.
func (a *apiImpl) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if !IsAdminFromContext(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req AdminCreateUserRequest
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			if isHTMXRequest(r) {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<p class="error">Invalid request body.</p>`))
				return
			}
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.User = r.FormValue("user")
	}
	if req.User == "" {
		http.Error(w, `{"error":"user is required"}`, http.StatusBadRequest)
		return
	}

	userID := UserIDFromContext(r)
	if err := a.store.CreateUserNoPassword(req.User); err != nil {
		slog.Warn("Admin user creation failed", "user", req.User, "admin", userID, "error", err)
		http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Admin created user", "user", req.User, "admin", userID)

	if isHTMXRequest(r) {
		users, err := a.store.ListUsers()
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Failed to reload users.</p>`))
			return
		}
		var sb strings.Builder
		renderUsersList(&sb, users)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(sb.String()))
		return
	}

	w.WriteHeader(http.StatusCreated)
	msg := MessageResponse{
		Message: strPtr("user created"),
	}
	json.NewEncoder(w).Encode(msg)
}

// AdminListAPIKeys implements the admin list API keys endpoint.
func (a *apiImpl) AdminListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !IsAdminFromContext(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	keys, err := a.store.ListAllAPIKeys()
	if err != nil {
		slog.Error("Error listing API keys", "error", err)
		http.Error(w, `{"error":"failed to list API keys"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

// AdminCreateAPIKey implements the admin create API key endpoint.
func (a *apiImpl) AdminCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if !IsAdminFromContext(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req AdminCreateAPIKeyRequest
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if isHTMXRequest(r) {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<p class="error">Invalid request body.</p>`))
				return
			}
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			if isHTMXRequest(r) {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<p class="error">Invalid request body.</p>`))
				return
			}
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.UserId = r.FormValue("user_id")
		req.Name = strPtr(r.FormValue("name"))
		if *req.Name == "" {
			req.Name = nil
		}
	}
	if req.UserId == "" {
		if isHTMXRequest(r) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">User ID is required.</p>`))
			return
		}
		http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
		return
	}

	adminID := UserIDFromContext(r)

	// Generate a new API key
	rawKey, err := generateAPIKey()
	if err != nil {
		slog.Error("Error generating API key", "error", err)
		if isHTMXRequest(r) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Failed to generate API key.</p>`))
			return
		}
		http.Error(w, `{"error":"failed to generate API key"}`, http.StatusInternalServerError)
		return
	}

	keyHash := storage.HashAPIKey(rawKey)
	var nameStr string
	if req.Name != nil {
		nameStr = *req.Name
	}
	keyID, err := a.store.CreateAPIKey(req.UserId, adminID, keyHash, nameStr)
	if err != nil {
		slog.Error("Error creating API key", "error", err)
		if isHTMXRequest(r) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Failed to create API key.</p>`))
			return
		}
		http.Error(w, `{"error":"failed to create API key"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Admin created API key", "user_id", req.UserId, "admin", adminID, "name", nameStr)

	if isHTMXRequest(r) {
		keys, err := a.store.ListAllAPIKeys()
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Failed to reload keys.</p>`))
			return
		}
		resultHTML := fmt.Sprintf(`<div class="success"><strong>New API Key (shown once):</strong><br>%s<br><small>ID: %d, Name: %s</small></div>`,
			html.EscapeString(rawKey), keyID, html.EscapeString(nameStr))
		var sb strings.Builder
		sb.WriteString(resultHTML)
		renderKeyTableHTML(&sb, keys)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(sb.String()))
		return
	}

	resp := APIKeyResponse{
		KeyId:  int64Ptr(keyID),
		Secret: strPtr(rawKey),
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// AdminRevokeAPIKey implements the admin revoke API key endpoint.
func (a *apiImpl) AdminRevokeAPIKey(w http.ResponseWriter, r *http.Request, keyId int64) {
	if !IsAdminFromContext(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	adminID := UserIDFromContext(r)
	if err := a.store.RevokeAPIKey(keyId); err != nil {
		slog.Error("Error revoking API key", "key_id", keyId, "admin", adminID, "error", err)
		if isHTMXRequest(r) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Failed to revoke API key.</p>`))
			return
		}
		http.Error(w, `{"error":"failed to revoke API key"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Admin revoked API key", "key_id", keyId, "admin", adminID)

	if isHTMXRequest(r) {
		keys, err := a.store.ListAllAPIKeys()
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Failed to reload keys.</p>`))
			return
		}
		var sb strings.Builder
		renderKeyTableHTML(&sb, keys)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(sb.String()))
		return
	}

	msg := MessageResponse{
		Message: strPtr("API key revoked"),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

// handleAvatarUpload processes multipart avatar image upload.
// Accepts a single file in the "avatar" form field.
// Max file size: 2MB.
func (a *apiImpl) handleAvatarUpload(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r)
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Parse multipart form with max 2MB
	const maxUpload = 2 << 20 // 2 MB
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		slog.Warn("Avatar upload: multipart parse error", "error", err)
		http.Error(w, `{"error":"invalid upload — max 2MB"}`, http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("avatar")
	if err != nil {
		slog.Warn("Avatar upload: no file in 'avatar' field", "error", err)
		http.Error(w, `{"error":"no avatar file provided"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	pngBytes, err := processAvatar(file, maxUpload)
	if err != nil {
		slog.Warn("Avatar upload: processing failed", "error", err)
		http.Error(w, `{"error":"`+html.EscapeString(err.Error())+`"}`, http.StatusBadRequest)
		return
	}

	if err := a.store.SetAvatar(userID, pngBytes); err != nil {
		slog.Error("Avatar upload: store failed", "error", err)
		http.Error(w, `{"error":"failed to save avatar"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("Avatar uploaded", "user", userID, "size", len(pngBytes))

	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<p class="success">Profile picture updated.</p>`))
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleAvatarGet serves the current user's avatar as image/png.
func (a *apiImpl) handleAvatarGet(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r)
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	a.serveAvatar(w, r, userID)
}

// handleAvatarGetUser serves any user's avatar by userID from the URL path.
func (a *apiImpl) handleAvatarGetUser(w http.ResponseWriter, r *http.Request) {
	// Extract userID from path: /api/user/avatar/{userID}
	userID := strings.TrimPrefix(r.URL.Path, "/api/user/avatar/")
	if userID == "" {
		http.Error(w, `{"error":"user ID required"}`, http.StatusBadRequest)
		return
	}
	a.serveAvatar(w, r, userID)
}

func (a *apiImpl) serveAvatar(w http.ResponseWriter, r *http.Request, userID string) {
	avatar, err := a.store.GetAvatar(userID)
	if err != nil || len(avatar) == 0 {
		http.Error(w, `{"error":"no avatar"}`, http.StatusNotFound)
		return
	}

	// ETag based on SHA-256 of the avatar bytes
	sum := sha256.Sum256(avatar)
	etag := `"sha256-` + hex.EncodeToString(sum[:]) + `"`

	// Check If-None-Match for cache revalidation
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.Header().Set("ETag", etag)
	w.Write(avatar)
}
func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// renderKeyTableHTML renders an HTML table of API keys into the given string builder.
func renderKeyTableHTML(sb *strings.Builder, keys []storage.APIKeyInfoFull) {
	if len(keys) == 0 {
		sb.WriteString(`<p style="color: #888; margin-top: 0.5rem;">No API keys created.</p>`)
		return
	}

	sb.WriteString(`<table><thead><tr><th>ID</th><th>User</th><th>Name</th><th>Created</th><th></th></tr></thead><tbody>`)
	for _, k := range keys {
		ts := "unknown"
		if !k.CreatedAt.IsZero() {
			ts = k.CreatedAt.Format("2006-01-02 15:04")
		}
		nameStr := ""
		if k.Name != nil {
			nameStr = html.EscapeString(*k.Name)
		} else {
			nameStr = "—"
		}
		sb.WriteString(`<tr>`)
		sb.WriteString(fmt.Sprintf(`<td>%d</td>`, k.ID))
		sb.WriteString(fmt.Sprintf(`<td>%s</td>`, html.EscapeString(k.UserID)))
		sb.WriteString(fmt.Sprintf(`<td>%s</td>`, nameStr))
		sb.WriteString(fmt.Sprintf(`<td>%s</td>`, ts))
		sb.WriteString(fmt.Sprintf(`<td><button type="button" hx-delete="/api/admin/api-keys/%d" hx-target="#keys-table-container" hx-swap="innerHTML" hx-headers='{"Accept": "text/html"}' onclick="if(!confirm('Revoke this key?'))event.preventDefault()">Revoke</button></td>`, k.ID))
		sb.WriteString(`</tr>`)
	}
	sb.WriteString(`</tbody></table>`)
}

// renderUsersList renders an HTML list of users into the given string builder.
func renderUsersList(sb *strings.Builder, users []storage.UserInfo) {
	if len(users) == 0 {
		sb.WriteString(`<p style="color: #888; margin-top: 0.5rem;">No users found.</p>`)
		return
	}

	for _, u := range users {
		sb.WriteString(`<div class="user-row">`)
		sb.WriteString(fmt.Sprintf(`<span>%s</span>`, html.EscapeString(u.Username)))
		if u.IsAdmin {
			sb.WriteString(`<span class="badge admin">Admin</span>`)
		} else {
			sb.WriteString(`<span class="badge user">User</span>`)
		}
		sb.WriteString(`</div>`)
	}
}

// generateAPIKey generates a new API key with the gk_ prefix.
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "gk_" + hex.EncodeToString(bytes), nil
}
