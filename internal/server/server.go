package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/javilopezg/yups-proxy/internal/bot"
	"github.com/javilopezg/yups-proxy/internal/checker"
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
	checker  *checker.Checker
	renderer *ui.Renderer
	mux      *http.ServeMux
}

// New creates and configures a new Server instance.
func New(cfg Config, registry *proxy.Registry, cache *metadata.Cache, chk *checker.Checker) (*Server, error) {
	renderer, err := ui.NewRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ui renderer: %w", err)
	}

	s := &Server{
		config:   cfg,
		registry: registry,
		cache:    cache,
		checker:  chk,
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

	// Help and user guide
	s.mux.HandleFunc("/help", s.handleHelp)

	// Explicit proxy links endpoint
	s.mux.HandleFunc("/links", s.handleLinks)

	// Status page
	s.mux.HandleFunc("/status", s.handleStatus)

	// Main routing endpoint
	s.mux.HandleFunc("/", s.handleRoot)
}

// Handler returns the HTTP handler with logging middleware wrapped.
func (s *Server) Handler() http.Handler {
	recoveryWrapper := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intercept path recovery before http.ServeMux collapses double slashes or redirects
		if s.tryPathRecovery(w, r) {
			return
		}
		s.mux.ServeHTTP(w, r)
	})
	return s.loggingMiddleware(recoveryWrapper)
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

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	if rec, ok := w.(*responseRecorder); ok {
		rec.action = "HELP"
	}
	if err := s.renderer.RenderHelp(w, ui.HelpViewData{BaseURL: s.config.BaseURL}, http.StatusOK); err != nil {
		http.Error(w, "Failed to render help page", http.StatusInternalServerError)
	}
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	s.serveResultsOrRedirect(w, r, true)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if rec, ok := w.(*responseRecorder); ok {
		rec.action = "STATUS"
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data ui.StatusViewData
	if s.checker != nil {
		rep := s.checker.LastReport()
		if rep != nil {
			data = ui.StatusViewData{
				HasReport:          true,
				CheckedAtFormatted: rep.Summary.CheckedAt.Format("2006-01-02 15:04:05 UTC"),
				DurationFormatted:  fmt.Sprintf("%.2fs", rep.Summary.Duration.Seconds()),
				Total:              rep.Summary.Total,
				Active:             rep.Summary.Active,
				Inactive:           rep.Summary.Inactive,
				Skipped:            rep.Summary.Skipped,
				Fallback:           rep.Summary.Fallback,
				FallbackServices:   rep.Summary.FallbackServices,
				ByService:          rep.Summary.ByService,
				Results:            rep.Results,
			}
		}
	}

	if err := s.renderer.RenderStatus(w, data); err != nil {
		log.Printf("ERROR: failed to render status page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// 1. Non-root paths: try path recovery (e.g. /https://...) or render 404 help page
	if r.URL.Path != "/" {
		if s.tryPathRecovery(w, r) {
			return
		}
		if rec, ok := w.(*responseRecorder); ok {
			rec.action = "NOT_FOUND"
		}
		if err := s.renderer.RenderHelp(w, ui.HelpViewData{
			ErrorMessage: "The requested page or resource could not be found (404).",
			BaseURL:      s.config.BaseURL,
		}, http.StatusNotFound); err != nil {
			http.NotFound(w, r)
		}
		return
	}

	// 2. Query parameter recovery & handling on "/"
	query := r.URL.Query()
	rawURL := strings.TrimSpace(query.Get("url"))

	if rawURL != "" {
		// Case 4: Extra query parameters when url is present
		if hasExtraParams(query) {
			if rec, ok := w.(*responseRecorder); ok {
				rec.action = "RECOVERY_EXTRA_PARAMS"
			}
			http.Redirect(w, r, "/?url="+url.QueryEscape(rawURL)+"&action=links", http.StatusTemporaryRedirect)
			return
		}

		// Canonical request with url
		action := query.Get("action")
		view := query.Get("view")
		mode := query.Get("mode")
		isLinks := action == "links" || view == "links" || mode == "links"

		s.serveResultsOrRedirect(w, r, isLinks)
		return
	}

	// rawURL is empty: check if query string can be recovered (Cases 2 and 3)
	if s.tryQueryRecovery(w, r) {
		return
	}

	// If query was provided with unrecovered, non-ignored parameters, render help with 400 Bad Request
	if hasNonIgnoredParams(query, r.URL.RawQuery) {
		if rec, ok := w.(*responseRecorder); ok {
			rec.action = "INVALID_QUERY"
		}
		if err := s.renderer.RenderHelp(w, ui.HelpViewData{
			ErrorMessage: "We could not recognize a valid URL from your request. Check out the usage guide below.",
			BaseURL:      s.config.BaseURL,
		}, http.StatusBadRequest); err != nil {
			http.Error(w, "Invalid URL request", http.StatusBadRequest)
		}
		return
	}

	// Clean home page
	if rec, ok := w.(*responseRecorder); ok {
		rec.action = "HOME"
	}
	base := strings.TrimRight(s.config.BaseURL, "/")
	if base == "" {
		base = "https://yups.io"
	}
	prefix := base + "/?url="
	if err := s.renderer.RenderIndex(w, ui.IndexViewData{PrefixURL: prefix}); err != nil {
		http.Error(w, "Failed to render home page", http.StatusInternalServerError)
	}
}

func (s *Server) renderError(w http.ResponseWriter, msg string, statusCode int) {
	if err := s.renderer.RenderHelp(w, ui.HelpViewData{
		ErrorMessage: msg,
		BaseURL:      s.config.BaseURL,
	}, statusCode); err != nil {
		http.Error(w, msg, statusCode)
	}
}

func (s *Server) serveResultsOrRedirect(w http.ResponseWriter, r *http.Request, forceResults bool) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	normURL, err := proxy.NormalizeURL(rawURL)
	if err != nil {
		if rec, ok := w.(*responseRecorder); ok {
			rec.action = "INVALID_URL"
		}
		s.renderError(w, fmt.Sprintf("Invalid URL provided: %v", err), http.StatusBadRequest)
		return
	}

	isBot := bot.IsSocialBot(r.UserAgent())

	// Check if the input URL belongs to any configured proxy (active or inactive).
	// If so, revert to the canonical original URL, choose a proxy distinct from itself,
	// or fallback to the results page if no alternative proxy exists.
	if matchedProxy, isProxy := s.registry.FindProxy(normURL); isProxy {
		origURL, err := proxy.Revert(matchedProxy, normURL)
		if err != nil {
			if rec, ok := w.(*responseRecorder); ok {
				rec.action = "REVERT_ERROR"
			}
			s.renderError(w, fmt.Sprintf("Failed to revert proxy URL: %v", err), http.StatusBadRequest)
			return
		}

		normOrigURL, err := proxy.NormalizeURL(origURL)
		if err != nil {
			if rec, ok := w.(*responseRecorder); ok {
				rec.action = "REVERT_ERROR"
			}
			s.renderError(w, fmt.Sprintf("Invalid reverted URL: %v", err), http.StatusBadRequest)
			return
		}

		if isBot || forceResults {
			s.serveResultsPage(w, r, normOrigURL, isBot)
			return
		}

		// Find active proxy candidates for the service, excluding the matched proxy itself.
		_, candidates := s.registry.MatchService(normOrigURL)
		var distinct []proxy.Entry
		for _, cand := range candidates {
			if !proxy.IsSameProxy(cand, matchedProxy) {
				distinct = append(distinct, cand)
			}
		}

		// If no distinct active proxy exists, fallback to the results page instead of self-redirecting.
		if len(distinct) == 0 {
			s.serveResultsPage(w, r, normOrigURL, isBot)
			return
		}

		if rec, ok := w.(*responseRecorder); ok {
			rec.action = "REDIRECT"
		}
		picked, err := proxy.PickRandom(distinct)
		if err != nil {
			s.renderError(w, "No proxy available", http.StatusBadGateway)
			return
		}

		destURL, err := proxy.Transform(picked, normOrigURL)
		if err != nil {
			s.renderError(w, "Failed to generate proxy redirect", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Vary", "User-Agent")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		http.Redirect(w, r, destURL, http.StatusTemporaryRedirect)
		return
	}

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
		s.renderError(w, "No proxy available", http.StatusBadGateway)
		return
	}

	destURL, err := proxy.Transform(picked, normURL)
	if err != nil {
		s.renderError(w, "Failed to generate proxy redirect", http.StatusInternalServerError)
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

	service, candidates := s.registry.MatchServiceAll(normURL)

	var activeLinks, manualLinks, inactiveLinks []ui.ProxyLink
	for _, cand := range candidates {
		dest, err := proxy.Transform(cand, normURL)
		if err != nil {
			continue
		}

		var status, statusLabel, statusClass string
		if cand.Active && cand.AutoCheck {
			status = "active"
			statusLabel = "Active"
			statusClass = "status-active"
		} else if !cand.AutoCheck {
			status = "manual"
			statusLabel = "Manual"
			statusClass = "status-manual"
		} else {
			status = "inactive"
			statusLabel = "Inactive"
			statusClass = "status-inactive"
		}

		link := ui.ProxyLink{
			Tech:           cand.Tech,
			Description:    cand.Description,
			DestinationURL: dest,
			Status:         status,
			StatusLabel:    statusLabel,
			StatusClass:    statusClass,
		}

		switch status {
		case "active":
			activeLinks = append(activeLinks, link)
		case "manual":
			manualLinks = append(manualLinks, link)
		case "inactive":
			inactiveLinks = append(inactiveLinks, link)
		}
	}

	allProxies := make([]ui.ProxyLink, 0, len(activeLinks)+len(manualLinks)+len(inactiveLinks))
	allProxies = append(allProxies, activeLinks...)
	allProxies = append(allProxies, manualLinks...)
	allProxies = append(allProxies, inactiveLinks...)

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
		Proxies:            allProxies,
		ActiveProxies:      activeLinks,
		ManualProxies:      manualLinks,
		InactiveProxies:    inactiveLinks,
	}

	if err := s.renderer.RenderResults(w, data); err != nil {
		http.Error(w, "Failed to render preview", http.StatusInternalServerError)
	}
}

func (s *Server) tryPathRecovery(w http.ResponseWriter, r *http.Request) bool {
	reqURI := r.RequestURI
	if strings.HasPrefix(reqURI, "http://") || strings.HasPrefix(reqURI, "https://") {
		if u, err := url.Parse(reqURI); err == nil {
			reqURI = u.RequestURI()
		}
	}
	raw := strings.TrimPrefix(reqURI, "/")
	if raw == "" {
		raw = strings.TrimPrefix(r.URL.Path, "/")
	}
	if raw == "" {
		return false
	}

	lowerRaw := strings.ToLower(raw)

	// Exclude known endpoints and static routes
	firstSegment := lowerRaw
	if idx := strings.Index(firstSegment, "/"); idx != -1 {
		firstSegment = firstSegment[:idx]
	}
	switch firstSegment {
	case "help", "healthz", "links", "status", "static", "favicon.ico", "robots.txt":
		return false
	}

	// Fix collapsed slashes if needed (e.g. http:/ -> http:// or https:/ -> https://)
	if strings.HasPrefix(lowerRaw, "http:/") && !strings.HasPrefix(lowerRaw, "http://") {
		raw = "http://" + raw[6:]
	} else if strings.HasPrefix(lowerRaw, "https:/") && !strings.HasPrefix(lowerRaw, "https://") {
		raw = "https://" + raw[7:]
	} else if strings.HasPrefix(lowerRaw, "http%3a") || strings.HasPrefix(lowerRaw, "https%3a") {
		if unescaped, err := url.PathUnescape(raw); err == nil {
			raw = unescaped
		}
	}

	if !isValidTargetURL(raw) {
		return false
	}

	// If the target doesn't have an explicit scheme, prepend https:// for the canonical URL
	target := raw
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}

	if rec, ok := w.(*responseRecorder); ok {
		rec.action = "RECOVERY_PATH"
	}
	http.Redirect(w, r, "/?url="+url.QueryEscape(target), http.StatusTemporaryRedirect)
	return true
}

func (s *Server) tryQueryRecovery(w http.ResponseWriter, r *http.Request) bool {
	rawQuery := strings.TrimSpace(r.URL.RawQuery)
	if rawQuery == "" {
		return false
	}

	// Case 2: Query string without parameter name e.g. ?https://twitter.com or ?twitter.com/user or ?https://youtube.com/watch?v=123
	lowerQuery := strings.ToLower(rawQuery)
	isRawURLQuery := !strings.Contains(rawQuery, "=") ||
		strings.HasPrefix(lowerQuery, "http://") ||
		strings.HasPrefix(lowerQuery, "https://") ||
		strings.HasPrefix(lowerQuery, "http%3a") ||
		strings.HasPrefix(lowerQuery, "https%3a")

	if isRawURLQuery {
		candidate := rawQuery
		if strings.HasPrefix(lowerQuery, "http%3a") || strings.HasPrefix(lowerQuery, "https%3a") {
			if unescaped, err := url.QueryUnescape(candidate); err == nil {
				candidate = unescaped
			}
		}
		if isValidTargetURL(candidate) {
			if rec, ok := w.(*responseRecorder); ok {
				rec.action = "RECOVERY_RAW_QUERY"
			}
			http.Redirect(w, r, "/?url="+url.QueryEscape(candidate), http.StatusTemporaryRedirect)
			return true
		}
	}

	// Case 3: Query parameter that is not "url", but "url" is missing e.g. ?u=laUrl
	query := r.URL.Query()
	if len(query) > 0 {
		var candidate string
		// Common parameter aliases
		aliases := []string{"u", "uri", "link", "target", "q", "dest", "destination", "address", "site", "href"}
		for _, alias := range aliases {
			if val := strings.TrimSpace(query.Get(alias)); val != "" {
				candidate = val
				break
			}
		}

		// Look for any parameter value matching a valid target URL
		if candidate == "" {
			for k, vals := range query {
				lk := strings.ToLower(k)
				if lk == "action" || lk == "view" || lk == "mode" || isIgnoredParam(lk) {
					continue
				}
				for _, v := range vals {
					v = strings.TrimSpace(v)
					if v != "" && isValidTargetURL(v) {
						candidate = v
						break
					}
				}
				if candidate != "" {
					break
				}
			}
		}

		// Look for first non-empty param value
		if candidate == "" {
			for k, vals := range query {
				lk := strings.ToLower(k)
				if lk == "action" || lk == "view" || lk == "mode" || isIgnoredParam(lk) {
					continue
				}
				for _, v := range vals {
					v = strings.TrimSpace(v)
					if v != "" {
						candidate = v
						break
					}
				}
				if candidate != "" {
					break
				}
			}
		}

		if candidate != "" && isValidTargetURL(candidate) {
			if rec, ok := w.(*responseRecorder); ok {
				rec.action = "RECOVERY_PARAM"
			}
			http.Redirect(w, r, "/?url="+url.QueryEscape(candidate), http.StatusTemporaryRedirect)
			return true
		}
	}

	return false
}

// ignoredParams contains known query parameters that should be ignored
// rather than treated as errors (e.g. analytics, search, pagination, sorting, IDs).
var ignoredParams = map[string]struct{}{
	"utm_source":           {},
	"utm_medium":           {},
	"utm_campaign":         {},
	"utm_term":             {},
	"utm_content":          {},
	"utm_id":               {},
	"utm_source_platform":  {},
	"utm_creative_format":  {},
	"utm_marketing_tactic": {},
	"query":                {},
	"search":               {},
	"filter":               {},
	"page":                 {},
	"p":                    {},
	"limit":                {},
	"size":                 {},
	"offset":               {},
	"sort":                 {},
	"order":                {},
	"orderby":              {},
	"direction":            {},
	"id":                   {},
	"userid":               {},
	"uuid":                 {},
}

func isIgnoredParam(param string) bool {
	_, ok := ignoredParams[strings.ToLower(param)]
	return ok
}

func hasExtraParams(query url.Values) bool {
	for k := range query {
		lk := strings.ToLower(k)
		switch lk {
		case "", "url", "action", "view", "mode":
			// Allowed canonical parameters
		default:
			if isIgnoredParam(lk) {
				continue
			}
			return true
		}
	}
	return false
}

// hasNonIgnoredParams checks whether the query contains any parameters that
// are neither empty nor part of the known ignored parameters list.
func hasNonIgnoredParams(query url.Values, rawQuery string) bool {
	trimmed := strings.Trim(rawQuery, "& \t\r\n")
	if trimmed == "" {
		return false
	}
	if len(query) == 0 {
		return !isIgnoredParam(strings.ToLower(trimmed))
	}
	for k := range query {
		lk := strings.ToLower(k)
		if lk != "" && !isIgnoredParam(lk) {
			return true
		}
	}
	return false
}

func isValidTargetURL(raw string) bool {
	norm, err := proxy.NormalizeURL(raw)
	if err != nil {
		return false
	}
	u, err := url.Parse(norm)
	if err != nil || u.Host == "" {
		return false
	}
	hostname := u.Hostname()
	return strings.Contains(hostname, ".") || hostname == "localhost" || net.ParseIP(hostname) != nil
}
