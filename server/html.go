package server

import (
	"fmt"
	"hash/fnv"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"embed"

	"grunt/client"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// HandleIndexOrLogin serves the main index page or login page based on authentication.
// It handles both GET / and GET /login, as well as POST /login for form submissions.
func HandleIndexOrLogin(w http.ResponseWriter, r *http.Request) {
	// POST /login - handle login form submission
	if r.Method == http.MethodPost && r.URL.Path == "/login" {
		handleLoginSubmit(w, r)
		return
	}

	// GET / or GET /login - serve the appropriate page based on auth context
	if AuthenticatedFromContext(r) {
		// Authenticated, serve chat UI with initial messages
		messages, err := DefaultStore.Sync(0, 50) // Load last 50 messages
		if err != nil {
			http.Error(w, "Failed to load messages", http.StatusInternalServerError)
			return
		}

		// Render chat page with messages
		content, err := templateFS.ReadFile("templates/chat.html")
		if err != nil {
			http.Error(w, "Template not found", http.StatusInternalServerError)
			return
		}

		// Replace placeholders with message HTML, admin status, and profile avatar
		messageHTML := renderMessages(messages)
		html := string(content)
		html = strings.Replace(html, "{{.Messages}}", messageHTML, 1)
		if IsAdminFromContext(r) {
			html = strings.Replace(html, "{{.Admin}}", "true", 1)
		} else {
			html = strings.Replace(html, "{{.Admin}}", "false", 1)
		}

		// Replace profile icon with hash-based avatar
		userID := UserIDFromContext(r)
		if userID != "" {
			html = strings.Replace(html, `<a href="/settings" class="profile-icon"></a>`,
				`<a href="/settings"><div class="avatar">`+generateAvatarSVG(userID)+`</div></a>`, 1)
		}

		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Write([]byte(html))
	} else {
		// Not authenticated, serve login form
		content, err := templateFS.ReadFile("templates/login.html")
		if err != nil {
			http.Error(w, "Template not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(content)
	}
}

// avatarColor returns a deterministic HSL color string from the given userID.
func avatarColor(userID string) string {
	h := fnv.New32a()
	h.Write([]byte(userID))
	hash := h.Sum32()

	hue := float64(hash % 360)
	saturation := 70.0 + float64(hash%25)
	lightness := 35.0 + float64((hash>>8)%31)

	return "hsl(" + strconv.FormatFloat(hue, 'f', -1, 32) + "," + strconv.FormatFloat(saturation, 'f', -1, 32) + "%," + strconv.FormatFloat(lightness, 'f', -1, 32) + "%)"
}

// avatarTextColor returns "#fff" or "#000" based on HSL lightness for optimal contrast.
func avatarTextColor(userID string) string {
	h := fnv.New32a()
	h.Write([]byte(userID))
	hash := h.Sum32()
	lightness := 35.0 + float64((hash>>8)%31)

	if lightness > 50 {
		return "#000"
	}
	return "#fff"
}

// generateAvatarSVG produces a deterministic avatar SVG from the given userID.
func generateAvatarSVG(userID string) string {
	initial := "?"
	if len(userID) > 0 {
		runeSlice := []rune(userID)
		initial = strings.ToUpper(string(runeSlice[0]))
	}

	return `<svg width="32" height="32" viewBox="0 0 32 32"><circle cx="16" cy="16" r="15.5" fill="` + avatarColor(userID) + `" /><text x="16" y="16" text-anchor="middle" dominant-baseline="central" fill="` + avatarTextColor(userID) + `" font-size="18" font-weight="bold" font-family="system-ui, sans-serif">` + html.EscapeString(initial) + `</text></svg>`
}

// renderMessages converts a slice of broadcasts to HTML.
func renderMessages(msgs []client.Broadcast) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(`<div class="message-row">`)
		sb.WriteString(`<div class="avatar-col">`)
		sb.WriteString(`<div class="avatar">` + generateAvatarSVG(m.UserID) + `</div>`)
		sb.WriteString(`<span class="timestamp">` + m.Timestamp.Format("15:04") + `</span>`)
		sb.WriteString(`</div>`)
		sb.WriteString(`<div class="message-bubble" data-id="` + fmt.Sprintf("%d", m.ID) + `">`)
		sb.WriteString(`<strong class="username" style="color: ` + avatarColor(m.UserID) + `">` + html.EscapeString(m.UserID) + `</strong>`)
		sb.WriteString(`<p>` + html.EscapeString(m.Content) + `</p>`)
		sb.WriteString(`</div></div>`)
	}
	return sb.String()
}

// renderMessageHTML converts a single broadcast message to an HTML fragment.
func renderMessageHTML(m client.Broadcast) string {
	var sb strings.Builder
	sb.WriteString(`<div class="message-row">`)
	sb.WriteString(`<div class="avatar-col">`)
	sb.WriteString(`<div class="avatar">` + generateAvatarSVG(m.UserID) + `</div>`)
	sb.WriteString(`<span class="timestamp">` + m.Timestamp.Format("15:04") + `</span>`)
	sb.WriteString(`</div>`)
	sb.WriteString(`<div class="message-bubble" data-id="` + fmt.Sprintf("%d", m.ID) + `">`)
	sb.WriteString(`<strong class="username" style="color: ` + avatarColor(m.UserID) + `">` + html.EscapeString(m.UserID) + `</strong>`)
	sb.WriteString(`<p>` + html.EscapeString(m.Content) + `</p>`)
	sb.WriteString(`</div></div>`)
	return sb.String()
}

// handleLoginSubmit processes login form submissions.
func handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	user := r.FormValue("user")
	password := r.FormValue("password")

	if user == "" || password == "" {
		http.Error(w, `{"error":"user and password are required"}`, http.StatusBadRequest)
		return
	}

	// Verify credentials
	ok, err := DefaultStore.VerifyUser(user, password)
	if err != nil {
		http.Error(w, `{"error":"failed to verify user"}`, http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Generate token
	token, err := DefaultHub.GenerateToken(user)
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// Set cookie for API requests
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to chat page
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleRegisterSubmit processes registration form submissions.
func handleRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	user := r.FormValue("user")
	password := r.FormValue("password")
	inviteCode := r.FormValue("invite_code")

	if user == "" || password == "" || inviteCode == "" {
		http.Error(w, `{"error":"user, password, and invite code are required"}`, http.StatusBadRequest)
		return
	}

	// Validate invite code
	valid, err := DefaultStore.ValidateInvite(inviteCode)
	if err != nil {
		http.Error(w, `{"error":"failed to validate invite code"}`, http.StatusInternalServerError)
		return
	}
	if !valid {
		http.Error(w, `{"error":"invalid or expired invite code"}`, http.StatusUnauthorized)
		return
	}

	// Create user
	if err := DefaultStore.CreateUser(user, password); err != nil {
		// Check if user already exists
		http.Error(w, `{"error":"user already exists"}`, http.StatusConflict)
		return
	}

	// Mark invite as used
	if err := DefaultStore.MarkInviteUsed(inviteCode, user); err != nil {
		slog.Error("Error marking invite as used", "error", err)
		// Don't fail registration if invite marking fails
	}

	// Generate token for auto-login after registration
	token, err := DefaultHub.GenerateToken(user)
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// Set cookie for API requests
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to chat page
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleSettings serves the settings page for authenticated users.
func HandleSettings(w http.ResponseWriter, r *http.Request) {
	if !AuthenticatedFromContext(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	content, err := templateFS.ReadFile("templates/settings.html")
	if err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	html := string(content)
	if IsAdminFromContext(r) {
		html = strings.Replace(html, "{{.Admin}}", "true", 1)
	} else {
		html = strings.Replace(html, "{{.Admin}}", "false", 1)
	}

	// Replace profile icon with hash-based avatar linking back to chat
	userID := UserIDFromContext(r)
	if userID != "" {
		html = strings.Replace(html, `<a href="/" class="profile-icon"></a>`,
			`<a href="/"><div class="avatar">`+generateAvatarSVG(userID)+`</div></a>`, 1)
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// HandleIndex serves the main index page.
// It checks for authentication and serves either the chat UI or redirects to login.
func HandleIndex(w http.ResponseWriter, r *http.Request) {
	if !AuthenticatedFromContext(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	content, err := templateFS.ReadFile("templates/chat.html")
	if err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// HandleLogin serves the login page and handles login submissions.
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to chat
	if AuthenticatedFromContext(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if r.Method == http.MethodGet {
		content, err := templateFS.ReadFile("templates/login.html")
		if err != nil {
			http.Error(w, "Template not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(content)
		return
	}

	if r.Method == http.MethodPost {
		handleLoginSubmit(w, r)
		return
	}
}

// HandleStatic serves static files from the embedded static directory.
func HandleStatic(w http.ResponseWriter, r *http.Request) {
	// Extract the file path from the URL (remove "/static/" prefix)
	// With Go 1.22+ pattern matching, the path is in r.URL.Path
	filePath := strings.TrimPrefix(r.URL.Path, "/static/")

	if filePath == "" || filePath == "/" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	content, err := staticFS.ReadFile("static/" + filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Set appropriate content type based on file extension
	switch {
	case strings.HasSuffix(filePath, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	case strings.HasSuffix(filePath, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(filePath, ".html"):
		w.Header().Set("Content-Type", "text/html")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Write(content)
}
