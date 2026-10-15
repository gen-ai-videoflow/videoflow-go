package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"videoflow-go/internal/cloud"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cfg, err := cloud.EnvConfig()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	store, err := cloud.OpenStore(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if cfg.DevMode {
		err = store.Migrate(ctx)
	} else {
		err = store.CheckMigrations(ctx)
		if err == nil {
			err = store.CheckRuntimeRole(ctx)
		}
	}
	if err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	worker, err := cloud.NewWorker(ctx, store, cfg)
	if err != nil {
		slog.Error("worker configuration failed", "error", err)
		os.Exit(1)
	}
	if err = worker.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
