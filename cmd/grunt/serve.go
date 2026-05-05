package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"grunt/internal/server"
	"grunt/internal/storage"
)

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntP("port", "p", 54765, "Port to listen on")
}

var serveCmd = &cobra.Command{
	Use:   "serve [db_path]",
	Short: "Start the grunt server",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Setup dual logging: JSON to file, text to stdout
		logFile, err := os.OpenFile("server.json.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("Failed to open log file", "error", err)
			os.Exit(1)
		}
		defer logFile.Close()

		jsonHandler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo})
		textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
		dualHandler := &dualHandler{text: textHandler, json: jsonHandler}
		slog.SetDefault(slog.New(dualHandler))

		dbPath := ""
		if len(args) > 0 {
			dbPath = args[0]
		}

		store, err := storage.New(dbPath)
		if err != nil {
			slog.Error("Failed to create storage", "error", err)
			os.Exit(1)
		}
		defer store.Close()

		port, _ := cmd.Flags().GetInt("port")
srv := server.NewWithPort(store, port)

		if err := srv.Serve(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}

		slog.Info("Server stopped.")
	},
}

type dualHandler struct {
	text slog.Handler
	json slog.Handler
}

func (d *dualHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return d.text.Enabled(ctx, level) || d.json.Enabled(ctx, level)
}

func (d *dualHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	if err := d.text.Handle(ctx, r); err != nil {
		errs = append(errs, err)
	}
	r2 := r.Clone()
	if err := d.json.Handle(ctx, r2); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (d *dualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &dualHandler{
		text: d.text.WithAttrs(attrs),
		json: d.json.WithAttrs(attrs),
	}
}

func (d *dualHandler) WithGroup(name string) slog.Handler {
	return &dualHandler{
		text: d.text.WithGroup(name),
		json: d.json.WithGroup(name),
	}
}