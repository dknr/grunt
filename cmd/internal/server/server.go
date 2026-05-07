package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"grunt/client"
	"grunt/cmd/internal/storage"
)

type Server struct {
	hub      *Hub
	store    *storage.Store
	mux      *http.ServeMux
	httpSrv  *http.Server
}

func New(store *storage.Store) *Server {
	return NewWithPort(store, 54765)
}

func NewWithPort(store *storage.Store, port int) *Server {
	hub := NewHub(store)

	s := &Server{
		hub:   hub,
		store: store,
		mux:   http.NewServeMux(),
	}

	s.setupRoutes()

	go hub.Run()

	// Setup HTTP server for graceful shutdown
	s.httpSrv = &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("POST /user", s.handleRegister)
	s.mux.HandleFunc("POST /auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /ws", s.hub.HandleWebSocket)
	s.mux.HandleFunc("GET /sync", s.handleSync)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Registration attempt", "user", req.User, "reason", "invalid request body")
		http.Error(w, `{"error":"missing user or password field"}`, http.StatusBadRequest)
		return
	}
	if req.User == "" || req.Password == "" {
		slog.Warn("Registration attempt", "user", req.User, "reason", "missing fields")
		http.Error(w, `{"error":"user and password are required"}`, http.StatusBadRequest)
		return
	}
	if err := s.store.CreateUser(req.User, req.Password); err != nil {
		slog.Error("Error creating user", "error", err)
		http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		return
	}
	slog.Info("User registered", "user", req.User)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "user created"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"missing user or password field"}`, http.StatusBadRequest)
		return
	}
	ok, err := s.store.VerifyUser(req.User, req.Password)
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
	token, err := s.hub.GenerateToken(req.User)
	if err != nil {
		slog.Error("Error generating token", "error", err)
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}
	slog.Info("User logged in", "user", req.User)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	since := 0
	if sinceStr != "" {
		var err error
		since, err = strconv.Atoi(sinceStr)
		if err != nil {
			http.Error(w, `{"error":"invalid since parameter"}`, http.StatusBadRequest)
			return
		}
	}

	lastStr := r.URL.Query().Get("last")
	last := 0
	if lastStr != "" {
		var err error
		last, err = strconv.Atoi(lastStr)
		if err != nil {
			http.Error(w, `{"error":"invalid last parameter"}`, http.StatusBadRequest)
			return
		}
	}

	msgs, err := s.store.Sync(since, last)
	if err != nil {
		slog.Error("Error syncing messages", "error", err)
		http.Error(w, `{"error":"failed to sync messages"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
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

		s.store.Close()
	}()

	slog.Info("Starting grunt server", "addr", s.httpSrv.Addr)

	return s.httpSrv.ListenAndServe()
}

// SendSyncResponse sends a sync response to a websocket client
func (s *Server) SendSyncResponse(conn *websocket.Conn, msgs []client.Broadcast) error {
	data, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

}