// cloud-rewards runs bounded reconciliation, calculations and approved-policy
// confirmation, and materializes vesting. It never sends provider transfers.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	"6eek-go/internal/cloud"
)

func main() {
	cfg, err := cloud.EnvConfig()
	if err != nil {
		slog.Error("configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := cloud.OpenStore(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if err = store.CheckMigrations(ctx); err != nil {
		slog.Error("migrations", "error", err)
		os.Exit(1)
	}
	if !cfg.DevMode {
		if err = store.CheckRuntimeRole(ctx); err != nil {
			slog.Error("runtime role", "error", err)
			os.Exit(1)
		}
	}
	cursor := ""
	for {
		pass, stop := context.WithTimeout(ctx, 45*time.Second)
		next, failures, e := store.ReleaseRewardPage(pass, cursor)
		if maintenanceErr := store.RunRewardMaintenance(pass, cfg.RewardsEnabled, cfg.CashPayoutsEnabled); maintenanceErr != nil {
			slog.Error("reward maintenance requires review", "error", maintenanceErr)
		}
		stop()
		if e != nil {
			slog.Error("vesting pass", "error", e)
		}
		for _, failure := range failures {
			slog.Error("vesting account requires retry", "error", failure)
		}
		delay := time.Minute
		if e == nil {
			cursor = next
			if cursor != "" {
				delay = time.Second
			} else {
				delay = 15 * time.Minute
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
