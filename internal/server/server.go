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
	return NewWithPort(store, 54765)
}

func NewWithPort(store *storage.Store, port int) *Server {
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
		Addr:         ":" + strconv.Itoa(port),
		Handler:      s.r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) setupRoutes() {
	// User registration
	s.r.POST("/user", func(c *gin.Context) {
		var req struct {
			User string `json:"user"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing user field"})
			return
		}
		if err := s.store.CreateUser(req.User); err != nil {
			slog.Error("Error creating user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"message": "user created"})
	})

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

		lastStr := c.Query("last")
		last := 0
		if lastStr != "" {
			var err error
			last, err = strconv.Atoi(lastStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid last parameter"})
				return
			}
		}

		msgs, err := s.store.Sync(since, last)
		if err != nil {
			slog.Error("Error syncing messages", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync messages"})
			return
		}

		c.JSON(http.StatusOK, msgs)
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

		s.store.Close()
	}()

	slog.Info("Starting grunt server", "addr", s.httpSrv.Addr)

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