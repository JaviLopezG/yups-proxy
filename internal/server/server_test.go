package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/javilopezg/yups-proxy/internal/checker"
	"github.com/javilopezg/yups-proxy/internal/metadata"
	"github.com/javilopezg/yups-proxy/internal/proxy"
)

func setupTestServer(t *testing.T) *Server {
	reg := proxy.NewRegistry()
	if err := reg.LoadDefault(); err != nil {
		t.Fatalf("failed to load default proxies: %v", err)
	}

	cache := metadata.NewCache(1*time.Hour, nil)
	srv, err := New(Config{AccessLog: false}, reg, cache, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return srv
}

func TestHomePage(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for home page, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "YUPS") {
		t.Errorf("expected body to contain 'YUPS'")
	}
	if !strings.Contains(body, "I'm feeling lucky") {
		t.Errorf("expected body to contain 'I\\'m feeling lucky'")
	}
	if !strings.Contains(body, "Get proxy links") {
		t.Errorf("expected body to contain 'Get proxy links'")
	}
	if !strings.Contains(body, "https://github.com/javilopezg/yups-proxy") {
		t.Errorf("expected footer github link")
	}
	if !strings.Contains(body, "mail@javilopezg.com") {
		t.Errorf("expected footer email link")
	}
}

func TestNormalUserRedirect(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/?url=https://x.com/Wikipedia", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected HTTP 307 Temporary Redirect, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatalf("expected non-empty Location header")
	}
	if !strings.Contains(location, "/Wikipedia") {
		t.Errorf("expected location to contain '/Wikipedia', got %q", location)
	}

	vary := rec.Header().Get("Vary")
	if !strings.Contains(vary, "User-Agent") {
		t.Errorf("expected Vary: User-Agent header, got %q", vary)
	}
}

func TestSocialBotPreview(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/?url=https://x.com/Wikipedia", nil)
	req.Header.Set("User-Agent", "TelegramBot (like TwitterBot)")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for social bot, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "og:title") {
		t.Errorf("expected body to contain OpenGraph title tag")
	}
	if !strings.Contains(body, "Available Proxy Mirrors") {
		t.Errorf("expected body to list available proxies")
	}

	vary := rec.Header().Get("Vary")
	if !strings.Contains(vary, "User-Agent") {
		t.Errorf("expected Vary: User-Agent header")
	}
}

func TestUserExplicitLinks(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/?url=https://reddit.com/r/golang&action=links", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for links view, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Available Proxy Mirrors") {
		t.Errorf("expected body to contain proxy mirrors")
	}
}

func TestResultsPageCategoryOrdering(t *testing.T) {
	reg := proxy.NewRegistry()
	csvData := `service,tech,type,proxy_url,patterns,description,active,auto-check
twitter,active_mirror,domain_replace,https://active.example.com/,x.com,Active Mirror,true,true
twitter,manual_mirror,domain_replace,https://manual.example.com/,x.com,Manual Mirror,true,false
twitter,inactive_mirror,domain_replace,https://inactive.example.com/,x.com,Inactive Mirror,false,true
`
	if err := reg.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("failed to load csv: %v", err)
	}
	cache := metadata.NewCache(1*time.Hour, nil)
	chk := checker.New(reg, 5*time.Minute, 1*time.Second, 2)
	srv, err := New(Config{AccessLog: false}, reg, cache, chk)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/?url=https://x.com/Wikipedia&action=links", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Active Proxies") {
		t.Errorf("expected Active Proxies section")
	}
	if !strings.Contains(body, "Manual Mode Proxies") {
		t.Errorf("expected Manual Mode Proxies section")
	}
	if !strings.Contains(body, "Inactive Proxies") {
		t.Errorf("expected Inactive Proxies section")
	}

	activeIdx := strings.Index(body, "active_mirror")
	manualIdx := strings.Index(body, "manual_mirror")
	inactiveIdx := strings.Index(body, "inactive_mirror")

	if activeIdx == -1 || manualIdx == -1 || inactiveIdx == -1 {
		t.Fatalf("expected all 3 mirrors in body, got indices: %d, %d, %d", activeIdx, manualIdx, inactiveIdx)
	}

	if activeIdx >= manualIdx {
		t.Errorf("expected active mirror (idx %d) to appear BEFORE manual mirror (idx %d)", activeIdx, manualIdx)
	}
	if manualIdx >= inactiveIdx {
		t.Errorf("expected manual mirror (idx %d) to appear BEFORE inactive mirror (idx %d)", manualIdx, inactiveIdx)
	}
}

func TestHealthCheck(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /healthz, got %d", rec.Code)
	}
	if rec.Body.String() != "OK" {
		t.Errorf("expected 'OK', got %q", rec.Body.String())
	}
}

func TestStaticFiles(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/static/logo.png", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /static/logo.png, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected image/png content type, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestHelpPage(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /help, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "How to Use YUPS") {
		t.Errorf("expected help page to contain 'How to Use YUPS'")
	}
	if !strings.Contains(body, "https://yups.io/?url=") {
		t.Errorf("expected help page to contain 'https://yups.io/?url='")
	}
	if !strings.Contains(body, "Home") {
		t.Errorf("expected help page to contain Home link")
	}
}

func TestNotFoundHelpPage(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/non-existent-page", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for non-existent path, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "404") {
		t.Errorf("expected 404 notice in body")
	}
	if !strings.Contains(body, "How to Use YUPS") {
		t.Errorf("expected help guide on 404 page")
	}
}

func TestMalformedURLHelpPage(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/?url=http://", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for malformed URL, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid URL provided") {
		t.Errorf("expected invalid URL notice in body, got: %s", body)
	}
}

func TestErrorRecoveryPath(t *testing.T) {
	srv := setupTestServer(t)

	tests := []struct {
		name         string
		path         string
		wantLocation string
	}{
		{
			name:         "full https path",
			path:         "/https://x.com/Wikipedia",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia",
		},
		{
			name:         "collapsed slash https path",
			path:         "/https:/x.com/Wikipedia",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia",
		},
		{
			name:         "full http path",
			path:         "/http://reddit.com/r/golang",
			wantLocation: "/?url=http%3A%2F%2Freddit.com%2Fr%2Fgolang",
		},
		{
			name:         "bare domain path",
			path:         "/x.com/Wikipedia",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia",
		},
		{
			name:         "bare domain path with subpath",
			path:         "/reddit.com/r/golang",
			wantLocation: "/?url=https%3A%2F%2Freddit.com%2Fr%2Fgolang",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusTemporaryRedirect {
				t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
			}
			location := rec.Header().Get("Location")
			if location != tt.wantLocation {
				t.Errorf("got location %q, want %q", location, tt.wantLocation)
			}
		})
	}
}

func TestErrorRecoveryRawQuery(t *testing.T) {
	srv := setupTestServer(t)

	tests := []struct {
		name         string
		query        string
		wantLocation string
	}{
		{
			name:         "raw https query",
			query:        "/?https://x.com/Wikipedia",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia",
		},
		{
			name:         "raw domain query without scheme",
			query:        "/?x.com/Wikipedia",
			wantLocation: "/?url=x.com%2FWikipedia",
		},
		{
			name:         "raw query with internal equals",
			query:        "/?https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			wantLocation: "/?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DdQw4w9WgXcQ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusTemporaryRedirect {
				t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
			}
			location := rec.Header().Get("Location")
			if location != tt.wantLocation {
				t.Errorf("got location %q, want %q", location, tt.wantLocation)
			}
		})
	}
}

func TestErrorRecoveryParam(t *testing.T) {
	srv := setupTestServer(t)

	tests := []struct {
		name         string
		query        string
		wantLocation string
	}{
		{
			name:         "param u",
			query:        "/?u=https://x.com/Wikipedia",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia",
		},
		{
			name:         "param q",
			query:        "/?q=https://reddit.com/r/golang",
			wantLocation: "/?url=https%3A%2F%2Freddit.com%2Fr%2Fgolang",
		},
		{
			name:         "param link without scheme",
			query:        "/?link=x.com/Wikipedia",
			wantLocation: "/?url=x.com%2FWikipedia",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusTemporaryRedirect {
				t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
			}
			location := rec.Header().Get("Location")
			if location != tt.wantLocation {
				t.Errorf("got location %q, want %q", location, tt.wantLocation)
			}
		})
	}
}

func TestErrorRecoveryExtraParams(t *testing.T) {
	srv := setupTestServer(t)

	tests := []struct {
		name         string
		query        string
		wantLocation string
	}{
		{
			name:         "UrlWithExtraQParam",
			query:        "/?url=https://x.com/Wikipedia&q=algo",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia&action=links",
		},
		{
			name:         "UrlWithUnknownParam",
			query:        "/?url=https://x.com/Wikipedia&custom_param=123",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia&action=links",
		},
		{
			name:         "UrlWithIgnoredAndUnknownParam",
			query:        "/?url=https://x.com/Wikipedia&utm_source=twitter&unknown_param=xyz",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia&action=links",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusTemporaryRedirect {
				t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
			}
			location := rec.Header().Get("Location")
			if location != tt.wantLocation {
				t.Errorf("got location %q, want %q", location, tt.wantLocation)
			}
		})
	}
}

func TestIgnoredParamsWithURL(t *testing.T) {
	srv := setupTestServer(t)

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "UtmTrackingParams",
			query: "/?url=https://x.com/Wikipedia&utm_source=twitter&utm_medium=feed&utm_campaign=promo",
		},
		{
			name:  "ExtendedUtmParams",
			query: "/?url=https://x.com/Wikipedia&utm_term=wiki&utm_content=btn&utm_id=1&utm_source_platform=web&utm_creative_format=card&utm_marketing_tactic=retargeting",
		},
		{
			name:  "PaginationAndSortingParams",
			query: "/?url=https://x.com/Wikipedia&page=2&p=3&limit=25&size=10&offset=20&sort=desc&order=asc&direction=next",
		},
		{
			name:  "SearchAndIdentityParams",
			query: "/?url=https://x.com/Wikipedia&query=go&search=1&filter=all&id=99&uuid=123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:  "CamelCaseParams",
			query: "/?url=https://x.com/Wikipedia&orderBy=name&userId=user-100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusTemporaryRedirect {
				t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
			}
			location := rec.Header().Get("Location")
			if strings.Contains(location, "action=links") {
				t.Errorf("expected direct proxy redirect, got recovery links location: %q", location)
			}
			if !strings.Contains(location, "/Wikipedia") {
				t.Errorf("expected location to contain '/Wikipedia', got %q", location)
			}
		})
	}
}

func TestIgnoredParamsWithoutURL(t *testing.T) {
	srv := setupTestServer(t)

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "UtmOnly",
			query: "/?utm_source=twitter&utm_medium=social&utm_campaign=launch",
		},
		{
			name:  "PaginationOnly",
			query: "/?page=2&limit=50",
		},
		{
			name:  "SearchAndFilterOnly",
			query: "/?query=yups&search=yups&filter=all",
		},
		{
			name:  "SortingAndCamelCaseOnly",
			query: "/?sort=asc&orderBy=date&userId=42&uuid=550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for clean home page with ignored params, got %d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "id=\"urlPrefix\"") {
				t.Errorf("expected body to contain home page id='urlPrefix'")
			}
		})
	}
}

func TestUnknownParamsWithoutURL(t *testing.T) {
	srv := setupTestServer(t)

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "UnknownSingleParam",
			query: "/?foo=bar",
		},
		{
			name:  "IgnoredPlusUnknownParam",
			query: "/?utm_source=twitter&unknown_extra=test",
		},
		{
			name:  "ActionWithoutURL",
			query: "/?action=links",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request for unknown params, got %d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "could not recognize a valid URL") {
				t.Errorf("expected invalid query notice in body, got: %s", body)
			}
		})
	}
}

func TestHomePagePrefixAndCopy(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id=\"urlPrefix\"") {
		t.Errorf("expected body to contain id='urlPrefix'")
	}
	if !strings.Contains(body, "id=\"copyBtn\"") {
		t.Errorf("expected body to contain id='copyBtn'")
	}
	if !strings.Contains(body, "https://yups.io/?url=") {
		t.Errorf("expected body to contain default prefix 'https://yups.io/?url='")
	}
}

func TestStatusPageNoReport(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /status, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "YUPS Proxy Status") {
		t.Errorf("expected body to contain 'YUPS Proxy Status'")
	}
	if !strings.Contains(body, "Initial Health Check in Progress") {
		t.Errorf("expected body to contain 'Initial Health Check in Progress'")
	}
	if !strings.Contains(body, "auto-refresh-toggle") {
		t.Errorf("expected body to contain auto-refresh-toggle")
	}
	if !strings.Contains(body, "countdown-badge") {
		t.Errorf("expected body to contain countdown-badge")
	}
}

func TestStatusPageWithChecker(t *testing.T) {
	reg := proxy.NewRegistry()
	if err := reg.LoadDefault(); err != nil {
		t.Fatalf("failed to load default proxies: %v", err)
	}
	cache := metadata.NewCache(1*time.Hour, nil)
	chk := checker.New(reg, 5*time.Minute, 1*time.Second, 2)

	report := &checker.Report{
		Summary: checker.Summary{
			CheckedAt:        time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
			Duration:         2500 * time.Millisecond,
			Total:            3,
			Active:           2,
			Inactive:         0,
			Skipped:          1,
			Fallback:         1,
			FallbackServices: []string{"twitter"},
			ByService: []checker.ServiceCounts{
				{Service: "twitter", Active: 1, Inactive: 0, Skipped: 0, Fallback: 1},
				{Service: "scribe", Active: 0, Inactive: 0, Skipped: 1, Fallback: 0},
			},
		},
		Results: []checker.CheckResult{
			{
				Index:      1,
				Service:    "twitter",
				Tech:       "nitter",
				TestURL:    "https://xcancel.com/Wikipedia",
				Status:     "200 OK (350ms)",
				IsActive:   true,
				IsFallback: true,
			},
			{
				Index:     2,
				Service:   "scribe",
				Tech:      "scribe",
				TestURL:   "https://scribe.r4fo.com",
				Status:    "manual / skipped",
				IsSkipped: true,
			},
		},
	}
	chk.SetReportForTesting(report)

	srv, err := New(Config{AccessLog: false}, reg, cache, chk)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /status, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "YUPS Proxy Status") {
		t.Errorf("expected body to contain 'YUPS Proxy Status'")
	}
	if !strings.Contains(body, "Proxy Health Summary") {
		t.Errorf("expected body to contain 'Proxy Health Summary'")
	}
	if !strings.Contains(body, "Detailed Check Results") {
		t.Errorf("expected body to contain 'Detailed Check Results'")
	}
	if !strings.Contains(body, "[PASS*]") {
		t.Errorf("expected body to contain '[PASS*]' tag")
	}
	if !strings.Contains(body, "[SKIP]") {
		t.Errorf("expected body to contain '[SKIP]' tag")
	}

	// Verify inverted order: Proxy Health Summary must appear BEFORE Detailed Check Results
	summaryIdx := strings.Index(body, "Proxy Health Summary")
	resultsIdx := strings.Index(body, "Detailed Check Results")
	if summaryIdx == -1 || resultsIdx == -1 || summaryIdx >= resultsIdx {
		t.Errorf("expected summary (idx %d) to appear BEFORE results (idx %d) in HTML", summaryIdx, resultsIdx)
	}

	// Verify test URLs are clickable links in the log
	expectedLink1 := `<a href="https://xcancel.com/Wikipedia" target="_blank" rel="noopener noreferrer" class="term-link">https://xcancel.com/Wikipedia</a>`
	expectedLink2 := `<a href="https://scribe.r4fo.com" target="_blank" rel="noopener noreferrer" class="term-link">https://scribe.r4fo.com</a>`
	if !strings.Contains(body, expectedLink1) {
		t.Errorf("expected body to contain clickable link: %s", expectedLink1)
	}
	if !strings.Contains(body, expectedLink2) {
		t.Errorf("expected body to contain clickable link: %s", expectedLink2)
	}
}

func TestProxyURLReversionAndRedirection(t *testing.T) {
	srv := setupTestServer(t)

	t.Run("ActiveProxyRedirectsToDistinctProxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?url=https://nitter.kareem.one/jack/status/123", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
		}
		location := rec.Header().Get("Location")
		if location == "" {
			t.Fatalf("expected non-empty Location header")
		}
		if strings.Contains(location, "nitter.kareem.one") {
			t.Errorf("expected redirect to a distinct proxy, got itself: %s", location)
		}
		if !strings.Contains(location, "/jack/status/123") {
			t.Errorf("expected location to preserve path /jack/status/123, got: %s", location)
		}
	})

	t.Run("InactiveProxyRevertsAndRedirectsToActiveProxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?url=https://nitter.net/jack/status/456", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
		}
		location := rec.Header().Get("Location")
		if location == "" {
			t.Fatalf("expected non-empty Location header")
		}
		if strings.Contains(location, "nitter.net") {
			t.Errorf("expected redirect away from inactive proxy nitter.net, got: %s", location)
		}
		if !strings.Contains(location, "/jack/status/456") {
			t.Errorf("expected location to preserve path /jack/status/456, got: %s", location)
		}
	})

	t.Run("SingleProxyServiceFallbacksToResultsPage", func(t *testing.T) {
		// ENS has only 1 gateway (eth.limo). When given eth.limo, there is no distinct proxy,
		// so it must display the results page instead of redirecting.
		req := httptest.NewRequest(http.MethodGet, "/?url=https://vitalik.eth.limo/about", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK (results page) when no distinct proxy exists, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Available Proxy Mirrors") {
			t.Errorf("expected results page to contain 'Available Proxy Mirrors'")
		}
	})

	t.Run("ProxyURLWithActionLinksShowsResults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?url=https://nitter.kareem.one/jack/status/123&action=links", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for action=links, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Available Proxy Mirrors") {
			t.Errorf("expected results page")
		}
	})

	t.Run("RevertErrorRendersErrorPage", func(t *testing.T) {
		// Create a server with an entry that will fail reversion (wildcard pattern for domain_replace)
		customReg := proxy.NewRegistry()
		csvData := `service,tech,type,proxy_url,patterns,description,active,auto-check
broken,broken_tech,domain_replace,https://broken.example.com/,*.invalid,Broken Proxy,true,true
`
		if err := customReg.LoadFromReader(strings.NewReader(csvData)); err != nil {
			t.Fatalf("failed to load broken csv: %v", err)
		}
		cache := metadata.NewCache(1*time.Hour, nil)
		brokenSrv, err := New(Config{AccessLog: false}, customReg, cache, nil)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/?url=https://broken.example.com/item/1", nil)
		rec := httptest.NewRecorder()

		brokenSrv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request on revert error, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Failed to revert proxy URL") {
			t.Errorf("expected error page with revert error message, got: %s", body)
		}
	})
}
