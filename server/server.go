package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"grunt/server/storage"
)

// Package-level variables for accessing store and hub from other handlers
var DefaultStore *storage.Store
var DefaultHub *Hub

type Server struct {
	hub      *Hub
	store    *storage.Store
	mux      *http.ServeMux
	httpSrv  *http.Server
	apiImpl  *apiImpl
}

func NewWithPort(dbPath string, port int) *Server {
	store, err := storage.New(dbPath)
	if err != nil {
		slog.Error("Failed to create storage", "error", err)
		os.Exit(1)
	}

	// Cold-start: if no users exist, generate an initial invite code
	count, err := store.CreateUserCount()
	if err != nil {
		slog.Error("Failed to count users", "error", err)
		os.Exit(1)
	}
	if count == 0 {
		code := os.Getenv("GRUNT_INITIAL_INVITE")
		if code == "" {
			code = generateInviteCode()
		}
		expiresAt := time.Now().Add(10 * time.Minute)
		if err := store.CreateInvite(code, expiresAt, ""); err != nil {
			slog.Error("Failed to create initial invite", "error", err)
			os.Exit(1)
		}
		slog.Warn("Initial invite code generated (no users exist yet)", "invite_code", code, "expires_at", expiresAt.Format(time.RFC3339))
	}

	hub := NewHub(store)
	apiImpl := &apiImpl{store: store, hub: hub}

	// Set package-level variables for access from other handlers
	DefaultStore = store
	DefaultHub = hub

	mux := http.NewServeMux()
	HandlerWithOptions(apiImpl, StdHTTPServerOptions{BaseRouter: mux})

	s := &Server{
		hub:   hub,
		store: store,
		mux:   mux,
		apiImpl: apiImpl,
	}

	s.setupRoutes()

	go hub.Run()

	// Setup HTTP server for graceful shutdown
	s.httpSrv = &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      authMiddleware(s.mux, s.store),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // 0 = no timeout (required for SSE long-lived connections)
		IdleTimeout:  0, // 0 = no timeout (required for SSE long-lived connections)
	}

	return s
}

func (s *Server) setupRoutes() {
	// SSE stream route is registered by the OpenAPI-generated HandlerWithOptions.

	// Serve the main index page and login page
	s.mux.HandleFunc("/", HandleIndexOrLogin)

	// Serve registration form submission
	s.mux.HandleFunc("/register", handleRegisterSubmit)

	// Serve settings page
	s.mux.HandleFunc("/settings", HandleSettings)

	// Serve logout
	s.mux.HandleFunc("/logout", handleLogoutSubmit)

	// Serve static files
	s.mux.HandleFunc("/static/", HandleStatic)

	// Serve runtime emotes from disk
	if emoteWatcher != nil {
		s.mux.HandleFunc("/emotes/", HandleRuntimeEmotes)
	}

	// Avatar routes
	s.mux.HandleFunc("POST /api/user/avatar", func(w http.ResponseWriter, r *http.Request) {
		s.apiImpl.handleAvatarUpload(w, r)
	})
	s.mux.HandleFunc("GET /api/user/avatar", func(w http.ResponseWriter, r *http.Request) {
		s.apiImpl.handleAvatarGet(w, r)
	})
	s.mux.HandleFunc("GET /api/user/avatar/", func(w http.ResponseWriter, r *http.Request) {
		s.apiImpl.handleAvatarGetUser(w, r)
	})
}

func (s *Server) Serve() error {
	// Graceful shutdown setup
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		slog.Info("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.httpSrv.Shutdown(ctx); err != nil {
			slog.Error("Server forced to shutdown", "error", err)
		}

		s.hub.Stop()
		s.store.Close()
		if emoteWatcher != nil {
			emoteWatcher.Close()
		}
	}()

	slog.Info("Starting grunt server", "addr", s.httpSrv.Addr)

	return s.httpSrv.ListenAndServe()
}
