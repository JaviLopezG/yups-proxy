package metadata

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", false},
		{"10.1.2.3", false},
		{"172.16.5.4", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false}, // AWS metadata
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"93.184.216.34", true}, // example.com
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			parsed := net.ParseIP(tt.ip)
			if parsed == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			got := IsPublicIP(parsed)
			if got != tt.expected {
				t.Errorf("IsPublicIP(%s) = %v, want %v", tt.ip, got, tt.expected)
			}
		})
	}
}

func TestExtractFromHTML(t *testing.T) {
	htmlContent := `
<!DOCTYPE html>
<html>
<head>
    <title>Original Page Title</title>
    <meta property="og:title" content="OpenGraph Title">
    <meta property="og:description" content="This is an awesome description of the page.">
    <meta property="og:image" content="https://example.com/cover.jpg">
    <meta property="og:site_name" content="AwesomeSite">
</head>
<body>
    <h1>Body Content</h1>
</body>
</html>`

	card, err := ExtractFromHTML(strings.NewReader(htmlContent), "https://example.com/article")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if card.Title != "OpenGraph Title" {
		t.Errorf("got title %q, want 'OpenGraph Title'", card.Title)
	}
	if card.Description != "This is an awesome description of the page." {
		t.Errorf("got description %q", card.Description)
	}
	if card.Image != "https://example.com/cover.jpg" {
		t.Errorf("got image %q", card.Image)
	}
	if card.SiteName != "AwesomeSite" {
		t.Errorf("got site name %q", card.SiteName)
	}
}

func TestExtractFromHTMLFallback(t *testing.T) {
	htmlContent := `
<!DOCTYPE html>
<html>
<head>
    <title>Simple Fallback Title</title>
    <meta name="description" content="Standard meta description">
</head>
<body>
</body>
</html>`

	card, err := ExtractFromHTML(strings.NewReader(htmlContent), "https://example.com/post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if card.Title != "Simple Fallback Title" {
		t.Errorf("got title %q, want 'Simple Fallback Title'", card.Title)
	}
	if card.Description != "Standard meta description" {
		t.Errorf("got description %q", card.Description)
	}
	if card.Image != "/static/logo.png" {
		t.Errorf("expected default image fallback, got %q", card.Image)
	}
}

func TestCacheRetention(t *testing.T) {
	cache := NewCache(1*time.Hour, nil)
	testURL := "https://example.com/test-article"

	initialCard := &CardData{
		URL:         testURL,
		Title:       "Initial Title",
		FirstSeenAt: time.Now().UTC().Add(-2 * time.Hour),
	}
	initialFirstSeen := initialCard.FirstSeenAt

	cache.Set(testURL, initialCard)

	cached, found := cache.Get(testURL)
	if !found {
		t.Fatalf("expected item to be in cache")
	}
	if cached.Title != "Initial Title" {
		t.Errorf("got title %q, want %q", cached.Title, "Initial Title")
	}

	// Update cache entry with new title
	updatedCard := &CardData{
		URL:   testURL,
		Title: "Updated Title",
	}
	cache.Set(testURL, updatedCard)

	retrieved, found := cache.Get(testURL)
	if !found {
		t.Fatalf("expected updated item in cache")
	}
	if retrieved.Title != "Updated Title" {
		t.Errorf("got title %q, want 'Updated Title'", retrieved.Title)
	}
	// FirstSeenAt MUST be preserved!
	if !retrieved.FirstSeenAt.Equal(initialFirstSeen) {
		t.Errorf("FirstSeenAt not preserved: got %v, want %v", retrieved.FirstSeenAt, initialFirstSeen)
	}
}

func TestSSRFBlocking(t *testing.T) {
	// Start a local test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	// Use safe HTTP client with SSRF protection
	client := NewSafeHTTPClient(1 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatalf("expected SSRF error when accessing loopback address %s, but got nil", ts.URL)
	}
}
