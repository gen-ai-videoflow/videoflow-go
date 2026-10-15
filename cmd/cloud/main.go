package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"videoflow-go/internal/cloud"
)

func main() {
	cfg, err := cloud.EnvConfig()
	if err != nil {
		slog.Error("cloud configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := cloud.OpenStore(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("cloud database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		slog.Error("cloud migrations", "error", err)
		os.Exit(1)
	}
	app := cloud.NewServer(store, cfg)
	defer app.Close()
	if cfg.OriginalsBucket != "" {
		app.Media, err = cloud.NewMedia(ctx, cfg)
		if err != nil {
			slog.Error("media configuration", "error", err)
			os.Exit(1)
		}
	}
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: app.Router(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	slog.Info("VideoFlow cloud API ready", "port", cfg.Port, "development", cfg.DevMode)
	if err = srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("cloud server stopped", "error", err)
		os.Exit(1)
	}
}
