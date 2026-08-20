package server

import (
	"bytes"
	"embed"
	"hash/fnv"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"grunt"
	"grunt/client"
	"grunt/server/storage"
)

// Template data structures
type AvatarData struct {
	UserID    string
	URL       string
	Color     string
	TextColor string
	Initial   string
	HasAvatar bool
}

type ChatData struct {
	Messages []MessageTemplateData
	Profile  AvatarData
}

type MessageTemplateData struct {
	ID              int
	User            string
	Content         string
	RenderedContent template.HTML
	Timestamp       string
	Color           string
	TextColor       string
	Initial         string
	HasAvatar       bool
	ShowAvatar      bool
	ShowUsername    bool
	ShowTimestamp   bool
	DateHeader      string // empty if no date divider needed
}

// Templates parsed in init()
var (
	loginTmpl    *template.Template
	chatTmpl     *template.Template
	settingsTmpl *template.Template
	messageTmpl  *template.Template // for SSE streaming in Hub
)

func init() {
	var err error

	// Login template is standalone
	loginTmpl, err = template.ParseFS(templateFS, "templates/login.html", "templates/partials/version.html")
	if err != nil {
		slog.Error("Failed to parse login template", "error", err)
		os.Exit(1)
	}

	// All other templates share a single namespace so they can reference each other
	allTemplates := []string{
		"templates/partials/avatar.html",
		"templates/partials/date-divider.html",
		"templates/partials/message.html",
		"templates/chat.html",
		"templates/settings.html",
		"templates/partials/version.html",
	}
	combined, err := template.ParseFS(templateFS, allTemplates...)
	if err != nil {
		slog.Error("Failed to parse combined templates", "error", err)
		os.Exit(1)
	}

	chatTmpl = combined.Lookup("chat.html")
	settingsTmpl = combined.Lookup("settings.html")
	messageTmpl = combined.Lookup("message.html")
}

//go:embed templates/login.html templates/chat.html templates/settings.html templates/partials/avatar.html templates/partials/date-divider.html templates/partials/message.html templates/partials/version.html
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

		// Build template data
		userID := UserIDFromContext(r)
		profile := AvatarData{
			UserID:    userID,
			URL:       "/settings",
			Color:     avatarColor(userID),
			TextColor: avatarTextColor(userID),
			Initial:   strings.ToUpper(string([]rune(userID)[0])),
			HasAvatar: DefaultStore.HasAvatar(userID),
		}

		msgs := make([]MessageTemplateData, len(messages))
		var lastMsgDate time.Time
		for i, m := range messages {
			showAvatar := true
			showUsername := true
			showTimestamp := true

			if i > 0 {
				prev := messages[i-1]
				if prev.UserID == m.UserID {
					showAvatar = false
					showUsername = false
					timeDiff := m.Timestamp.Sub(prev.Timestamp)
					if timeDiff <= time.Minute {
						showTimestamp = false
					}
				}
			}

			currentDate := time.Date(m.Timestamp.Year(), m.Timestamp.Month(), m.Timestamp.Day(), 0, 0, 0, 0, time.Local)
			var dateHeader string
			if i == 0 || !currentDate.Equal(lastMsgDate) {
				dateHeader = formatDateHeader(currentDate)
			}
			lastMsgDate = currentDate

			msgs[i] = MessageTemplateData{
				ID:              int(m.ID),
				User:            m.UserID,
				Content:         m.Content,
				RenderedContent: ReplaceEmotes(m.Content),
				Timestamp:       m.Timestamp.Format("15:04"),
				Color:           avatarColor(m.UserID),
				TextColor:       avatarTextColor(m.UserID),
				Initial:         strings.ToUpper(string([]rune(m.UserID)[0])),
				HasAvatar:       DefaultStore.HasAvatar(m.UserID),
				ShowAvatar:      showAvatar,
				ShowUsername:    showUsername,
				ShowTimestamp:   showTimestamp,
				DateHeader:      dateHeader,
			}
		}

		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		chatTmpl.Execute(w, ChatData{Messages: msgs, Profile: profile})
	} else {
		// Not authenticated, serve login form
		w.Header().Set("Content-Type", "text/html")
		loginTmpl.Execute(w, map[string]interface{}{"Version":    grunt.Version,
		"Timestamp":  grunt.Timestamp,
		"Commit":     grunt.Commit})
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

// formatDateHeader returns a human-readable date label for the given date,
// formatted in the server's local timezone.
func formatDateHeader(d time.Time) string {
	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	yesterday := today.AddDate(0, 0, -1)

	switch {
	case d.Equal(today):
		return "Today"
	case d.Equal(yesterday):
		return "Yesterday"
	default:
		return d.Format("Monday, January 2")
	}
}

// renderMessageHTMLTemplate renders a single broadcast message using the message template.
// This is used by the Hub for SSE HTML streaming.
func renderMessageHTMLTemplate(m client.Broadcast, showAvatar, showUsername, showTimestamp bool, dateHeader string) string {
	msg := MessageTemplateData{
		ID:              int(m.ID),
		User:            m.UserID,
		Content:         m.Content,
		RenderedContent: func() template.HTML {
			result := ReplaceEmotes(m.Content)
			return template.HTML(strings.ReplaceAll(string(result), "\n", "<br>"))
		}(),
		Timestamp:       m.Timestamp.Format("15:04"),
		Color:           avatarColor(m.UserID),
		TextColor:       avatarTextColor(m.UserID),
		Initial:         strings.ToUpper(string([]rune(m.UserID)[0])),
		HasAvatar:       DefaultStore.HasAvatar(m.UserID),
		ShowAvatar:      showAvatar,
		ShowUsername:    showUsername,
		ShowTimestamp:   showTimestamp,
		DateHeader:      dateHeader,
	}

	var buf bytes.Buffer
	if err := messageTmpl.ExecuteTemplate(&buf, "message", msg); err != nil {
		slog.Error("Failed to render message template", "error", err)
		return `<div class="message-row"><p>Error rendering message</p></div>`
	}

	return buf.String()
}

// handleLoginSubmit processes login form submissions.
func handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		loginTmpl.Execute(w, map[string]string{"Error": "Invalid request body"})
		return
	}

	user := r.FormValue("user")
	password := r.FormValue("password")

	if user == "" || password == "" {
		loginTmpl.Execute(w, map[string]string{"Error": "User and password are required"})
		return
	}

	// Verify credentials
	ok, err := DefaultStore.VerifyUser(user, password)
	if err != nil {
		slog.Error("Error verifying user", "error", err)
		loginTmpl.Execute(w, map[string]string{"Error": "Failed to verify user"})
		return
	}
	if !ok {
		slog.Warn("Failed login attempt", "user", user)
		loginTmpl.Execute(w, map[string]string{"Error": "Invalid credentials"})
		return
	}

	// Generate token
	token, err := DefaultHub.GenerateToken(user)
	if err != nil {
		slog.Error("Error generating token", "error", err)
		loginTmpl.Execute(w, map[string]string{"Error": "Failed to generate token"})
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
		loginTmpl.Execute(w, map[string]string{"Error": "Invalid request body"})
		return
	}

	user := r.FormValue("user")
	password := r.FormValue("password")
	inviteCode := r.FormValue("invite_code")

	if user == "" || password == "" || inviteCode == "" {
		loginTmpl.Execute(w, map[string]string{"Error": "User, password, and invite code are required"})
		return
	}

	// Atomically validate the invite, mark it used, and create the user
	_, err := DefaultStore.RegisterWithInvite(inviteCode, user, password)
	if err != nil {
		// Check if it's a duplicate user error
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			slog.Warn("User already registered", "user", user)
			loginTmpl.Execute(w, map[string]string{"Error": "User already exists"})
			return
		}
		slog.Warn("Registration failed", "user", user, "error", err)
		loginTmpl.Execute(w, map[string]string{"Error": "Invalid or expired invite code"})
		return
	}

	// Set first user as admin
	userCount, err := DefaultStore.CreateUserCount()
	if err == nil && userCount == 1 {
		if err := DefaultStore.SetUserAdmin(user); err != nil {
			slog.Error("Failed to set first user as admin", "error", err)
		} else {
			slog.Info("First user registered, granted admin privileges", "user", user)
		}
	}

	// Generate token for auto-login after registration
	token, err := DefaultHub.GenerateToken(user)
	if err != nil {
		slog.Error("Error generating token", "error", err)
		loginTmpl.Execute(w, map[string]string{"Error": "Failed to generate token"})
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

	userID := UserIDFromContext(r)
	isAdmin := IsAdminFromContext(r)
	profile := AvatarData{
		UserID:    userID,
		URL:       "/",
		Color:     avatarColor(userID),
		TextColor: avatarTextColor(userID),
		Initial:   strings.ToUpper(string([]rune(userID)[0])),
		HasAvatar: DefaultStore.HasAvatar(userID),
	}

	var users []storage.UserInfo
	var keys []storage.APIKeyInfoFull
	if isAdmin {
		var err error
		users, err = DefaultStore.ListUsers()
		if err != nil {
			slog.Error("Error listing users", "error", err)
			http.Error(w, "Failed to load users", http.StatusInternalServerError)
			return
		}
		keys, err = DefaultStore.ListAllAPIKeys()
		if err != nil {
			slog.Error("Error listing API keys", "error", err)
			http.Error(w, "Failed to load API keys", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html")
	settingsTmpl.Execute(w, map[string]interface{}{
		"Profile":    profile,
		"IsAdmin":    isAdmin,
		"Version":    grunt.Version,
		"Timestamp":  grunt.Timestamp,
		"Commit":     grunt.Commit,
		"Users":      users,
		"Keys":       keys,
	})
}

// handleLogoutSubmit processes logout requests.
func handleLogoutSubmit(w http.ResponseWriter, r *http.Request) {
	// Clear the token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	// Redirect to login page
	http.Redirect(w, r, "/login", http.StatusFound)
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
	case strings.HasSuffix(filePath, ".woff2"):
		w.Header().Set("Content-Type", "font/woff2")
	case strings.HasSuffix(filePath, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Write(content)
}
