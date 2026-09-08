package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/javilopezg/yups-proxy/internal/bot"
	"github.com/javilopezg/yups-proxy/internal/metadata"
	"github.com/javilopezg/yups-proxy/internal/proxy"
	"github.com/javilopezg/yups-proxy/internal/ui"
)

// Config configures the HTTP server instance.
type Config struct {
	Host      string
	Port      int
	BaseURL   string
	AccessLog bool
}

// Server encapsulates the YUPS HTTP services.
type Server struct {
	config   Config
	registry *proxy.Registry
	cache    *metadata.Cache
	renderer *ui.Renderer
	mux      *http.ServeMux
}

// New creates and configures a new Server instance.
func New(cfg Config, registry *proxy.Registry, cache *metadata.Cache) (*Server, error) {
	renderer, err := ui.NewRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ui renderer: %w", err)
	}

	s := &Server{
		config:   cfg,
		registry: registry,
		cache:    cache,
		renderer: renderer,
		mux:      http.NewServeMux(),
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	// Static files (logo and icons)
	staticHandler := http.StripPrefix("/static/", http.FileServer(ui.StaticFileSystem()))
	s.mux.Handle("/static/", staticHandler)

	// Health check
	s.mux.HandleFunc("/healthz", s.handleHealthz)

	// Explicit proxy links endpoint
	s.mux.HandleFunc("/links", s.handleLinks)

	// Main routing endpoint
	s.mux.HandleFunc("/", s.handleRoot)
}

// Handler returns the HTTP handler with logging middleware wrapped.
func (s *Server) Handler() http.Handler {
	return s.loggingMiddleware(s.mux)
}

// responseRecorder captures the status code for logging.
type responseRecorder struct {
	http.ResponseWriter
	status int
	action string
}

func (rec *responseRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
			action:         "REQUEST",
		}

		next.ServeHTTP(rec, r)

		if s.config.AccessLog {
			duration := time.Since(start)
			clientIP := getClientIP(r)
			targetURL := r.URL.Query().Get("url")
			if targetURL != "" && len(targetURL) > 80 {
				targetURL = targetURL[:77] + "..."
			}

			log.Printf("[YUPS] %s | %3d | %8v | %-15s | %-4s %-20s | action=%-12s | url=%q | ua=%q",
				start.Format("2006-01-02 15:04:05"),
				rec.status,
				duration.Round(time.Microsecond),
				clientIP,
				r.Method,
				r.URL.Path,
				rec.action,
				targetURL,
				r.UserAgent(),
			)
		}
	})
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		return ip[:idx]
	}
	return ip
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if rec, ok := w.(*responseRecorder); ok {
		rec.action = "HEALTH"
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	s.serveResultsOrRedirect(w, r, true)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// Only handle exact root "/"
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		if rec, ok := w.(*responseRecorder); ok {
			rec.action = "HOME"
		}
		if err := s.renderer.RenderIndex(w); err != nil {
			http.Error(w, "Failed to render home page", http.StatusInternalServerError)
		}
		return
	}

	// Determine if user explicitly requested links page
	action := r.URL.Query().Get("action")
	view := r.URL.Query().Get("view")
	mode := r.URL.Query().Get("mode")
	isLinks := action == "links" || view == "links" || mode == "links"

	s.serveResultsOrRedirect(w, r, isLinks)
}

func (s *Server) serveResultsOrRedirect(w http.ResponseWriter, r *http.Request, forceResults bool) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	normURL, err := proxy.NormalizeURL(rawURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid URL provided: %v", err), http.StatusBadRequest)
		return
	}

	isBot := bot.IsSocialBot(r.UserAgent())

	// Condition to show results page instead of redirect:
	// 1. Requester is a social media crawler (Telegram, Twitter, Discord, etc.)
	// 2. User clicked "Get proxy links" or navigated to /links
	if isBot || forceResults {
		s.serveResultsPage(w, r, normURL, isBot)
		return
	}

	// Normal user seeking direct redirection:
	s.serveTemporaryRedirect(w, r, normURL)
}

func (s *Server) serveTemporaryRedirect(w http.ResponseWriter, r *http.Request, normURL string) {
	if rec, ok := w.(*responseRecorder); ok {
		rec.action = "REDIRECT"
	}

	service, candidates := s.registry.MatchService(normURL)
	_ = service

	picked, err := proxy.PickRandom(candidates)
	if err != nil {
		http.Error(w, "No proxy available", http.StatusBadGateway)
		return
	}

	destURL, err := proxy.Transform(picked, normURL)
	if err != nil {
		http.Error(w, "Failed to generate proxy redirect", http.StatusInternalServerError)
		return
	}

	// Crucial requirements:
	// 1. Temporary redirect (307) so browsers do NOT cache the redirect
	// 2. Vary: User-Agent header
	// 3. Cache-Control: no-cache
	w.Header().Set("Vary", "User-Agent")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	http.Redirect(w, r, destURL, http.StatusTemporaryRedirect)
}

func (s *Server) serveResultsPage(w http.ResponseWriter, r *http.Request, normURL string, isBot bool) {
	if rec, ok := w.(*responseRecorder); ok {
		if isBot {
			rec.action = "BOT_PREVIEW"
		} else {
			rec.action = "LINKS_VIEW"
		}
	}

	// Retrieve or scrape smart card info
	card := s.cache.GetOrFetch(r.Context(), normURL)

	service, candidates := s.registry.MatchService(normURL)

	proxyLinks := make([]ui.ProxyLink, 0, len(candidates))
	for _, cand := range candidates {
		dest, err := proxy.Transform(cand, normURL)
		if err == nil {
			proxyLinks = append(proxyLinks, ui.ProxyLink{
				Tech:           cand.Tech,
				Description:    cand.Description,
				DestinationURL: dest,
			})
		}
	}

	w.Header().Set("Vary", "User-Agent")
	if isBot {
		// Cache social cards for 1 hour
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	data := ui.ResultsViewData{
		Service:            strings.ToUpper(service),
		Card:               card,
		FirstSeenFormatted: card.FirstSeenAt.Format("2006-01-02 15:04:05 UTC"),
		Proxies:            proxyLinks,
	}

	if err := s.renderer.RenderResults(w, data); err != nil {
		http.Error(w, "Failed to render preview", http.StatusInternalServerError)
	}
}
