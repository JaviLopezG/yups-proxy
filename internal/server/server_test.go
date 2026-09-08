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
