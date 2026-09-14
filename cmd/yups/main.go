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

	"github.com/javilopezg/yups-proxy/internal/checker"
	"github.com/javilopezg/yups-proxy/internal/metadata"
	"github.com/javilopezg/yups-proxy/internal/proxy"
	"github.com/javilopezg/yups-proxy/internal/server"
)

func main() {
	var (
		hostFlag          = flag.String("host", getEnv("HOST", "0.0.0.0"), "HTTP server host")
		portFlag          = flag.Int("port", getEnvInt("PORT", 8080), "HTTP server port")
		baseURLFlag       = flag.String("base-url", getEnv("BASE_URL", "https://yups.io"), "Public base URL of the service")
		proxiesFlag       = flag.String("proxies", getEnv("PROXIES_FILE", ""), "Path to custom proxies CSV file (default: embedded dataset)")
		accessLogFlag     = flag.Bool("access-log", getEnvBool("YUPS_ACCESS_LOG", true), "Enable HTTP access logging")
		cacheTTLFlag      = flag.Duration("cache-ttl", getEnvDuration("CACHE_TTL", 24*time.Hour), "TTL for smart card metadata cache")
		checkIntervalFlag = flag.Duration("check-interval", getEnvDuration("CHECK_INTERVAL", 5*time.Minute), "Interval between proxy health checks")
		checkTimeoutFlag  = flag.Duration("check-timeout", getEnvDuration("CHECK_TIMEOUT", 20*time.Second), "HTTP timeout for proxy health checks")
		checkWorkersFlag  = flag.Int("check-workers", getEnvInt("CHECK_WORKERS", 15), "Number of concurrent workers for proxy health checks")
		disableCheckFlag  = flag.Bool("disable-check", getEnvBool("DISABLE_CHECK", false), "Disable background proxy health checking")
		checkOnlyFlag     = flag.Bool("check", false, "Run one-shot proxy availability check and exit")
		updateCSVFlag     = flag.Bool("update-csv", false, "In -check mode, update proxies CSV file with check results")
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
	} else if *checkOnlyFlag && fileExists("data/proxies.csv") {
		log.Printf("Loading proxies from data/proxies.csv...")
		f, err := os.Open("data/proxies.csv")
		if err != nil {
			log.Fatalf("Failed to open data/proxies.csv: %v", err)
		}
		defer f.Close()
		if err := reg.LoadFromReader(f); err != nil {
			log.Fatalf("Failed to load proxies from data/proxies.csv: %v", err)
		}
	} else {
		log.Printf("Loading built-in proxy dataset...")
		if err := reg.LoadDefault(); err != nil {
			log.Fatalf("Failed to load default embedded proxies: %v", err)
		}
	}
	log.Printf("Loaded %d proxy configurations (%d active).", len(reg.Entries()), len(reg.ActiveEntries()))

	// One-shot check mode: verify proxies, print report to console, and exit
	if *checkOnlyFlag {
		log.Printf("Running one-shot proxy health check (timeout: %v, workers: %d)...", *checkTimeoutFlag, *checkWorkersFlag)
		chk := checker.New(reg, *checkIntervalFlag, *checkTimeoutFlag, *checkWorkersFlag)
		rep := chk.RunCheck(context.Background())
		checker.PrintReport(os.Stdout, rep)

		if *updateCSVFlag {
			csvPath := *proxiesFlag
			if csvPath == "" {
				csvPath = "data/proxies.csv"
			}
			if fileExists(csvPath) {
				if err := checker.UpdateCSV(csvPath, rep); err != nil {
					log.Fatalf("Failed to update %s: %v", csvPath, err)
				}
				log.Printf("Updated %s successfully.", csvPath)
			} else {
				log.Printf("Warning: CSV file %s not found on disk, skipping CSV update.", csvPath)
			}
		}
		return
	}

	// Initialize metadata cache with SSRF-safe HTTP client
	safeClient := metadata.NewSafeHTTPClient(3 * time.Second)
	cache := metadata.NewCache(*cacheTTLFlag, safeClient)

	// Initialize proxy checker
	var chk *checker.Checker
	if !*disableCheckFlag {
		chk = checker.New(reg, *checkIntervalFlag, *checkTimeoutFlag, *checkWorkersFlag)
	}

	// Create HTTP server
	cfg := server.Config{
		Host:      *hostFlag,
		Port:      *portFlag,
		BaseURL:   *baseURLFlag,
		AccessLog: *accessLogFlag,
	}
	srv, err := server.New(cfg, reg, cache, chk)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Start background proxy checker
	if chk != nil {
		chk.Start(context.Background())
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

	if chk != nil {
		chk.Stop()
	}

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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

