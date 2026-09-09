package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/javilopezg/yups-proxy/internal/metadata"
	"github.com/javilopezg/yups-proxy/internal/proxy"
)

func setupTestServer(t *testing.T) *Server {
	reg := proxy.NewRegistry()
	if err := reg.LoadDefault(); err != nil {
		t.Fatalf("failed to load default proxies: %v", err)
	}

	cache := metadata.NewCache(1*time.Hour, nil)
	srv, err := New(Config{AccessLog: false}, reg, cache)
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
	if !strings.Contains(body, "https://yups.io?url=") {
		t.Errorf("expected help page to contain 'https://yups.io?url='")
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
			name:         "url with extra q param",
			query:        "/?url=https://x.com/Wikipedia&q=algo",
			wantLocation: "/?url=https%3A%2F%2Fx.com%2FWikipedia&action=links",
		},
		{
			name:         "url with utm tracking params",
			query:        "/?url=https://x.com/Wikipedia&utm_source=twitter&utm_medium=feed",
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
	if !strings.Contains(body, "https://yups.io?url=") {
		t.Errorf("expected body to contain default prefix 'https://yups.io?url='")
	}
}
