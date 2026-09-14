package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/javilopezg/yups-proxy/internal/proxy"
)

func TestBuildTestURL(t *testing.T) {
	tests := []struct {
		entry    proxy.Entry
		expected string
	}{
		{
			entry: proxy.Entry{
				Service:  "youtube",
				Type:     "domain_replace",
				ProxyURL: "https://invidious.nerdvpn.de/",
			},
			expected: "https://invidious.nerdvpn.de/wikipedia",
		},
		{
			entry: proxy.Entry{
				Service:  "medium",
				Type:     "domain_replace",
				ProxyURL: "https://scribe.pussthecat.org/",
			},
			expected: "https://scribe.pussthecat.org/welcome-to-medium-9e53ca408c48",
		},
		{
			entry: proxy.Entry{
				Service:  "ens",
				Type:     "append_ext",
				ProxyURL: "https://eth.limo/",
			},
			expected: "https://zerolend.eth.limo/",
		},
		{
			entry: proxy.Entry{
				Service:  "i2p",
				Type:     "prepend",
				ProxyURL: "https://i2p.surf/proxy/",
			},
			expected: "https://i2p.surf/proxy/stormycloud.i2p/",
		},
		{
			entry: proxy.Entry{
				Service:  "general",
				Type:     "query_param",
				ProxyURL: "https://archive.is/submit/?url=",
			},
			expected: "https://archive.is/submit/?url=https%3A%2F%2Fwikipedia.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.entry.Service, func(t *testing.T) {
			got := BuildTestURL(tt.entry)
			if got != tt.expected {
				t.Errorf("BuildTestURL(%v) = %q, want %q", tt.entry.Service, got, tt.expected)
			}
		})
	}
}

func TestFindErrorInBody(t *testing.T) {
	tests := []struct {
		body     string
		expected string
	}{
		{"<h1>Error</h1>", "<h1>error"},
		{"<h1>error 404</h1>", "error 4"},
		{"<h2>Error: Something went wrong</h2>", "<h2>error"},
		{"<h3> Error </h3>", "<h3> Error"},
		{"<h4 class=\"error-title\">error</h4>", "<h4 class=\"error-title\">error"},
		{"Error 502 Bad Gateway", "error 5"},
		{"Welcome to Wikipedia user profile", ""},
		{"Normal page content", ""},
	}

	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			got := FindErrorInBody(strings.ToLower(tt.body))
			if tt.expected == "" && got != "" {
				t.Errorf("expected no error match, got %q", got)
			}
			if tt.expected != "" && got == "" {
				t.Errorf("expected error match for %q, got empty", tt.body)
			}
		})
	}
}

func TestEvaluateHealthyResponse(t *testing.T) {
	// 418 Teapot
	detail, ok := EvaluateHealthyResponse(418, http.Header{})
	if !ok || detail != "418 (Teapot)" {
		t.Errorf("expected 418 healthy, got %v, %s", ok, detail)
	}

	// 200 OK
	detail, ok = EvaluateHealthyResponse(200, http.Header{})
	if !ok || detail != "200" {
		t.Errorf("expected 200 healthy, got %v, %s", ok, detail)
	}

	// 500 Server Error
	_, ok = EvaluateHealthyResponse(500, http.Header{})
	if ok {
		t.Errorf("expected 500 unhealthy, got healthy")
	}

	// Cloudflare challenge active (cf-mitigated)
	hdr := http.Header{}
	hdr.Set("cf-mitigated", "challenge")
	detail, ok = EvaluateHealthyResponse(403, hdr)
	if !ok || detail != "403 (CF challenge)" {
		t.Errorf("expected cf-mitigated healthy, got %v, %s", ok, detail)
	}
}

func TestRunCheckWithMockServer(t *testing.T) {
	// Setup mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "healthy") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><body><h1>Wikipedia</h1><p>Content</p></body></html>"))
			return
		}
		if strings.Contains(r.URL.Path, "bodyerror") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><body><h1>Error 404</h1><p>Not found</p></body></html>"))
			return
		}
		if strings.Contains(r.URL.Path, "servererror") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 internal server error"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	csvContent := `service,tech,type,proxy_url,patterns,description,active,auto-check
cat-a,tech1,domain_replace,` + ts.URL + `/servererror/,a.com,A1,true,true
cat-a,tech2,domain_replace,` + ts.URL + `/bodyerror/,a.com,A2,true,true
cat-b,tech1,domain_replace,` + ts.URL + `/healthy/,b.com,B1,true,true
cat-b,tech2,domain_replace,` + ts.URL + `/servererror/,b.com,B2,true,true
cat-c,tech1,domain_replace,` + ts.URL + `/manual/,c.com,C1,false,false
cat-c,tech2,domain_replace,` + ts.URL + `/servererror/,c.com,C2,true,true
`

	reg := proxy.NewRegistry()
	if err := reg.LoadFromReader(strings.NewReader(csvContent)); err != nil {
		t.Fatalf("failed to load test csv: %v", err)
	}

	chk := New(reg, 5*time.Minute, 2*time.Second, 5)
	report := chk.RunCheck(context.Background())

	if report == nil {
		t.Fatalf("expected non-nil report")
	}

	if report.Summary.Total != 6 {
		t.Errorf("expected 6 total, got %d", report.Summary.Total)
	}

	// Cat-A: both probed failed -> fallback keeps both active!
	if !report.Results[0].IsActive || !report.Results[0].IsFallback {
		t.Errorf("expected Cat-A tech1 active via fallback, got %v, %v", report.Results[0].IsActive, report.Results[0].IsFallback)
	}
	if !report.Results[1].IsActive || !report.Results[1].IsFallback {
		t.Errorf("expected Cat-A tech2 active via fallback, got %v, %v", report.Results[1].IsActive, report.Results[1].IsFallback)
	}

	// Cat-B: 1 pass, 1 fail -> no fallback
	if !report.Results[2].IsActive || report.Results[2].IsFallback {
		t.Errorf("expected Cat-B tech1 healthy, got %v, %v", report.Results[2].IsActive, report.Results[2].IsFallback)
	}
	if report.Results[3].IsActive {
		t.Errorf("expected Cat-B tech2 inactive, got active")
	}

	// Cat-C: 1 manual kept false, 1 probed failed -> fallback keeps probed active
	if report.Results[4].IsActive || !report.Results[4].IsSkipped {
		t.Errorf("expected Cat-C tech1 skipped and inactive, got %v, %v", report.Results[4].IsActive, report.Results[4].IsSkipped)
	}
	if !report.Results[5].IsActive || !report.Results[5].IsFallback {
		t.Errorf("expected Cat-C tech2 active via fallback, got %v, %v", report.Results[5].IsActive, report.Results[5].IsFallback)
	}

	// Verify registry was updated live
	activeEntries := reg.ActiveEntries()
	// Cat-A (2) + Cat-B (1) + Cat-C (1) = 4 active entries
	if len(activeEntries) != 4 {
		t.Errorf("expected 4 active entries in registry after check, got %d", len(activeEntries))
	}
}
