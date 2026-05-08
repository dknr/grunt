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
	"grunt/server/storage"
)

type Server struct {
	hub      *Hub
	store    *storage.Store
	mux      *http.ServeMux
	httpSrv  *http.Server
}

func NewWithPort(dbPath string, port int) *Server {
	store, err := storage.New(dbPath)
	if err != nil {
		slog.Error("Failed to create storage", "error", err)
		os.Exit(1)
	}

	hub := NewHub(store)
	apiImpl := &apiImpl{store: store, hub: hub}

	mux := http.NewServeMux()
	HandlerWithOptions(apiImpl, StdHTTPServerOptions{BaseRouter: mux})

	s := &Server{
		hub:   hub,
		store: store,
		mux:   mux,
	}

	s.setupRoutes()

	go hub.Run()

	// Setup HTTP server for graceful shutdown
	s.httpSrv = &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      authMiddleware(s.mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("GET /ws", s.hub.HandleWebSocket)
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
