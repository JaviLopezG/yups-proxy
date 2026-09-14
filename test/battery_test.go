package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/javilopezg/yups-proxy/internal/metadata"
	"github.com/javilopezg/yups-proxy/internal/proxy"
	"github.com/javilopezg/yups-proxy/internal/server"
)

type serviceTestCase struct {
	Service            string
	TargetURL          string
	ExpectedDestSubstr string
}

var batteryServices = []serviceTestCase{
	{
		Service:            "Twitter",
		TargetURL:          "https://x.com/Wikipedia",
		ExpectedDestSubstr: "/Wikipedia",
	},
	{
		Service:            "YouTubeStandard",
		TargetURL:          "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		ExpectedDestSubstr: "v=dQw4w9WgXcQ",
	},
	{
		Service:            "YouTubeShortlink",
		TargetURL:          "https://youtu.be/dQw4w9WgXcQ",
		ExpectedDestSubstr: "/watch?v=dQw4w9WgXcQ",
	},
	{
		Service:            "Instagram",
		TargetURL:          "https://www.instagram.com/p/C-12345/",
		ExpectedDestSubstr: "/p/C-12345/",
	},
	{
		Service:            "Reddit",
		TargetURL:          "https://www.reddit.com/r/golang/comments/123",
		ExpectedDestSubstr: "/r/golang/comments/123",
	},
	{
		Service:            "Medium",
		TargetURL:          "https://medium.com/@author/sample-story",
		ExpectedDestSubstr: "/@author/sample-story",
	},
	{
		Service:            "Imgur",
		TargetURL:          "https://imgur.com/gallery/abcde",
		ExpectedDestSubstr: "/gallery/abcde",
	},
	{
		Service:            "Goodreads",
		TargetURL:          "https://www.goodreads.com/book/show/50",
		ExpectedDestSubstr: "/book/show/50",
	},
	{
		Service:            "ENS",
		TargetURL:          "https://vitalik.eth/about",
		ExpectedDestSubstr: ".eth.limo/about",
	},
	{
		Service:            "I2P",
		TargetURL:          "http://forum.i2p/thread/1",
		ExpectedDestSubstr: "i2p.surf/proxy/http://forum.i2p/thread/1",
	},
	{
		Service:            "GeneralFallback",
		TargetURL:          "https://news.ycombinator.com/item?id=42",
		ExpectedDestSubstr: "archive.",
	},
}

func setupBatteryServer(t *testing.T) *server.Server {
	reg := proxy.NewRegistry()
	if err := reg.LoadDefault(); err != nil {
		t.Fatalf("failed to load default proxies: %v", err)
	}

	cache := metadata.NewCache(1*time.Hour, nil)
	srv, err := server.New(server.Config{AccessLog: false}, reg, cache, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return srv
}

func TestBatteryServices(t *testing.T) {
	srv := setupBatteryServer(t)

	for _, tc := range batteryServices {
		t.Run(tc.Service, func(t *testing.T) {
			// 1. Normal user browser request
			t.Run("NormalUserRedirect", func(t *testing.T) {
				reqURL := "/?url=" + url.QueryEscape(tc.TargetURL)
				req := httptest.NewRequest(http.MethodGet, reqURL, nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
				rec := httptest.NewRecorder()

				srv.Handler().ServeHTTP(rec, req)

				if rec.Code != http.StatusTemporaryRedirect {
					t.Fatalf("expected HTTP 307 Temporary Redirect, got %d", rec.Code)
				}

				loc := rec.Header().Get("Location")
				if loc == "" {
					t.Fatalf("expected Location header in redirect")
				}
				if !strings.Contains(loc, tc.ExpectedDestSubstr) {
					t.Errorf("expected location %q to contain %q", loc, tc.ExpectedDestSubstr)
				}

				vary := rec.Header().Get("Vary")
				if !strings.Contains(vary, "User-Agent") {
					t.Errorf("missing Vary: User-Agent header")
				}
			})

			// 2. Social crawler bot request (Telegram, Twitter, Discord, etc.)
			t.Run("SocialBotPreview", func(t *testing.T) {
				reqURL := "/?url=" + url.QueryEscape(tc.TargetURL)
				req := httptest.NewRequest(http.MethodGet, reqURL, nil)
				req.Header.Set("User-Agent", "TelegramBot (like TwitterBot)")
				rec := httptest.NewRecorder()

				srv.Handler().ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200 OK for social bot, got %d", rec.Code)
				}

				body := rec.Body.String()
				if !strings.Contains(body, "og:title") {
					t.Errorf("expected OpenGraph og:title meta tag in bot preview")
				}
				if !strings.Contains(body, "Available Proxy Mirrors") {
					t.Errorf("expected proxy mirror listings in bot preview")
				}

				vary := rec.Header().Get("Vary")
				if !strings.Contains(vary, "User-Agent") {
					t.Errorf("missing Vary: User-Agent header")
				}
			})
		})
	}
}
