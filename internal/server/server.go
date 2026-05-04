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

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"grunt/internal/message"
	"grunt/internal/storage"
)

type Server struct {
	hub      *Hub
	store    *storage.Store
	r        *gin.Engine
	httpSrv  *http.Server
}

func New(store *storage.Store) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	hub := NewHub(store)

	s := &Server{
		hub:   hub,
		store: store,
		r:     r,
	}

	s.setupRoutes()

	go hub.Run()

	// Setup HTTP server for graceful shutdown
	s.httpSrv = &http.Server{
		Addr:         ":54765",
		Handler:      s.r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) setupRoutes() {
	// WebSocket endpoint
	s.r.GET("/ws", s.hub.HandleWebSocket)

	// Sync endpoint
	s.r.GET("/sync", func(c *gin.Context) {
		sinceStr := c.Query("since")
		since := 0
		if sinceStr != "" {
			var err error
			since, err = strconv.Atoi(sinceStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since parameter"})
				return
			}
		}

		msgs, err := s.store.Sync(since)
		if err != nil {
			slog.Error("Error syncing messages", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync messages"})
			return
		}

		c.JSON(http.StatusOK, msgs)
	})
}

func (s *Server) Serve() error {
	slog.Info("Starting grunt server", "addr", s.httpSrv.Addr)

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

	return s.httpSrv.ListenAndServe()
}

// SendSyncResponse sends a sync response to a websocket client
func (s *Server) SendSyncResponse(conn *websocket.Conn, msgs []message.Broadcast) error {
	data, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}