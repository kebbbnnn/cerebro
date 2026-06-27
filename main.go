package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[cerebro] ")

	// Determine config file path.
	configPath := os.Getenv("CEREBRO_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}

	// Load and validate configuration.
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded %d Cerebras API key(s) and %d tenant(s)", len(cfg.CerebrasKeys), len(cfg.Tenants))

	// Parse upstream URL.
	upstream, err := url.Parse(cfg.Server.Upstream)
	if err != nil {
		log.Fatalf("Invalid upstream URL %q: %v", cfg.Server.Upstream, err)
	}

	// Initialize core components.
	pool := NewKeyPool(cfg.CerebrasKeys, cfg.DefaultCooldownSeconds)
	stats := NewStatsCollector()

	// Create the reverse proxy handler.
	proxyHandler := NewProxyHandler(upstream, pool, stats)

	// Apply auth middleware to the proxy.
	authedProxy := AuthMiddleware(cfg.Tenants)(proxyHandler)

	// Set up routing.
	mux := http.NewServeMux()

	// Health check — no auth required.
	mux.HandleFunc("/health", HealthHandler(pool))

	// Stats — protected by tenant auth (any valid tenant can view).
	mux.Handle("/stats", AuthMiddleware(cfg.Tenants)(http.HandlerFunc(StatsHandler(pool, stats))))

	// Proxy all /v1/ requests — with tenant auth.
	mux.Handle("/v1/", authedProxy)

	// Catch-all: return helpful error for non-proxied paths.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"name":"cerebro","version":"1.0.0","status":"running"}`)
			return
		}
		// For any other path, return 404.
		http.NotFound(w, r)
	})

	// Create HTTP server.
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      logMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // Long timeout for streaming responses.
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine.
	go func() {
		log.Printf("Starting server on %s (upstream: %s)", addr, cfg.Server.Upstream)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received %v, shutting down gracefully...", sig)

	// Graceful shutdown with 30s timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Forced shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// logMiddleware logs incoming requests (method, path, duration).
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Skip logging for health checks to reduce noise.
		if strings.HasPrefix(r.URL.Path, "/health") {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
