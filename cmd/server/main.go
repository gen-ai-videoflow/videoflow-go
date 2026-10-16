package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"6eek-go/internal/agents"
	"6eek-go/internal/api"
	"6eek-go/internal/config"
	"6eek-go/internal/db"
	"6eek-go/internal/desktop"
	"6eek-go/internal/events"
	"6eek-go/internal/jobs"
	"6eek-go/internal/media"
	"6eek-go/internal/providers"
)

func main() {
	log.Println("Starting 6eek Go Backend Engine...")

	desktopMode := flag.Bool("desktop", false, "serve the bundled interface and keep projects in your user data folder")
	noBrowser := flag.Bool("no-browser", false, "do not open a browser on startup")
	uiDir := flag.String("ui-dir", "", "interface directory (desktop mode)")
	bindHost := flag.String("host", "127.0.0.1", "listen address")
	flag.Parse()
	cfg := config.Load()
	if *desktopMode {
		folder, err := desktop.Configure(cfg, *uiDir)
		if err != nil {
			log.Fatal(err)
		}
		*uiDir = folder
	}
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer database.Close()
	log.Printf("SQLite initialized at %s with WAL mode", cfg.DatabasePath)

	broker := events.NewBroker()
	mediaEngine := media.NewMediaEngine()
	if mediaEngine.Available() {
		log.Println("FFmpeg detected and available for media processing")
	} else {
		log.Println("Warning: FFmpeg not detected in PATH; thumbnails and clipping disabled")
	}

	openrouter := providers.NewOpenRouterClient(cfg.OpenRouterAPIKey)
	// All AI generation uses OpenRouter.
	agentsEngine := agents.NewAgentEngine(openrouter, "openai/gpt-4o-mini")

	workers := jobs.NewWorkerPool(cfg, database, broker, mediaEngine)
	workers.Start()
	workers.ReconcilePending()
	defer workers.Stop()

	server := api.NewServer(cfg, database, broker, workers, agentsEngine, openrouter, mediaEngine)
	defer server.Close()
	addr := net.JoinHostPort(*bindHost, fmt.Sprint(cfg.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Cannot start 6eek on %s: %v. Another copy may already be running.", addr, err)
	}
	// PORT=0 is used by Electron; guard and report the actual assigned port.
	addr = listener.Addr().String()
	handler := server.Router()
	if *desktopMode {
		handler = desktop.Handler(handler, *uiDir, addr)
	}
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("6eek ready at http://%s", addr)
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	if *desktopMode && !*noBrowser {
		go func() {
			if err := desktop.OpenBrowser("http://" + addr); err != nil {
				log.Printf("Open http://%s in your browser to use 6eek", addr)
			}
		}()
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server gracefully...")
	// Cancel background provider/media tasks before waiting for HTTP connections.
	server.Close()
	workers.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
		_ = httpServer.Close()
	}
	log.Println("6eek Go backend stopped.")
}
