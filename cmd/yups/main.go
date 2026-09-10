package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/javilopezg/yups-proxy/internal/metadata"
	"github.com/javilopezg/yups-proxy/internal/proxy"
	"github.com/javilopezg/yups-proxy/internal/server"
)

func main() {
	var (
		hostFlag      = flag.String("host", getEnv("HOST", "0.0.0.0"), "HTTP server host")
		portFlag      = flag.Int("port", getEnvInt("PORT", 8080), "HTTP server port")
		baseURLFlag   = flag.String("base-url", getEnv("BASE_URL", "https://yups.io"), "Public base URL of the service")
		proxiesFlag   = flag.String("proxies", getEnv("PROXIES_FILE", ""), "Path to custom proxies CSV file (default: embedded dataset)")
		accessLogFlag = flag.Bool("access-log", getEnvBool("YUPS_ACCESS_LOG", true), "Enable HTTP access logging")
		cacheTTLFlag  = flag.Duration("cache-ttl", getEnvDuration("CACHE_TTL", 24*time.Hour), "TTL for smart card metadata cache")
	)
	flag.Parse()

	log.Printf("Starting YUPS (Your Unified Proxy Service)...")

	// Initialize proxy registry
	reg := proxy.NewRegistry()
	if *proxiesFlag != "" {
		log.Printf("Loading proxies from custom file: %s", *proxiesFlag)
		f, err := os.Open(*proxiesFlag)
		if err != nil {
			log.Fatalf("Failed to open custom proxies file %q: %v", *proxiesFlag, err)
		}
		defer f.Close()
		if err := reg.LoadFromReader(f); err != nil {
			log.Fatalf("Failed to load proxies from %q: %v", *proxiesFlag, err)
		}
	} else {
		log.Printf("Loading built-in proxy dataset...")
		if err := reg.LoadDefault(); err != nil {
			log.Fatalf("Failed to load default embedded proxies: %v", err)
		}
	}
	log.Printf("Loaded %d proxy configurations (%d active).", len(reg.Entries()), len(reg.ActiveEntries()))

	// Initialize metadata cache with SSRF-safe HTTP client
	safeClient := metadata.NewSafeHTTPClient(3 * time.Second)
	cache := metadata.NewCache(*cacheTTLFlag, safeClient)

	// Create HTTP server
	cfg := server.Config{
		Host:      *hostFlag,
		Port:      *portFlag,
		BaseURL:   *baseURLFlag,
		AccessLog: *accessLogFlag,
	}
	srv, err := server.New(cfg, reg, cache)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	addr := net.JoinHostPort(*hostFlag, strconv.Itoa(*portFlag))
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Server shutdown listener
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("YUPS server listening on http://%s (Public URL: %s, AccessLog: %v)", addr, *baseURLFlag, *accessLogFlag)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-stopChan
	log.Println("Shutting down YUPS server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Error during server shutdown: %v", err)
	}
	log.Println("YUPS server gracefully stopped.")
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return fallback
}
