package proxy

import (
	"strings"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	reg := NewRegistry()
	if err := reg.LoadDefault(); err != nil {
		t.Fatalf("expected nil error loading default csv, got %v", err)
	}
	if len(reg.Entries()) == 0 {
		t.Fatalf("expected non-empty registry entries")
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "already valid https",
			input:    "https://x.com/user/status/123",
			expected: "https://x.com/user/status/123",
		},
		{
			name:     "missing scheme adds https",
			input:    "reddit.com/r/golang",
			expected: "https://reddit.com/r/golang",
		},
		{
			name:     "encoded url scheme",
			input:    "https%3A%2F%2Finstagram.com%2Fp%2F123",
			expected: "https://instagram.com/p/123",
		},
		{
			name:     "empty url errors",
			input:    "   ",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("got %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestMatchService(t *testing.T) {
	reg := NewRegistry()
	if err := reg.LoadDefault(); err != nil {
		t.Fatalf("failed to load default: %v", err)
	}

	tests := []struct {
		url             string
		expectedService string
	}{
		{"https://x.com/Wikipedia", "twitter"},
		{"https://twitter.com/jack/status/1", "twitter"},
		{"https://xcancel.com/Wikipedia", "twitter"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube"},
		{"https://youtu.be/dQw4w9WgXcQ", "youtube"},
		{"https://www.reddit.com/r/golang/comments/123", "reddit"},
		{"https://instagram.com/p/C-12345", "instagram"},
		{"https://medium.com/@author/story-slug", "medium"},
		{"https://imgur.com/gallery/abcde", "imgur"},
		{"https://goodreads.com/book/show/50", "goodreads"},
		{"https://vitalik.eth/blog", "ens"},
		{"http://trillian.i2p/forums", "i2p"},
		{"https://news.ycombinator.com/item?id=12345", "general"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedService+"_"+tt.url, func(t *testing.T) {
			svc, candidates := reg.MatchService(tt.url)
			if svc != tt.expectedService {
				t.Errorf("for url %q: got service %q, want %q", tt.url, svc, tt.expectedService)
			}
			if len(candidates) == 0 {
				t.Errorf("for url %q: expected at least one candidate", tt.url)
			}
		})
	}
}

func TestTransform(t *testing.T) {
	tests := []struct {
		name       string
		entry      Entry
		targetURL  string
		wantPrefix string
		wantSubstr string
	}{
		{
			name: "twitter domain replacement",
			entry: Entry{
				Service:  "twitter",
				Tech:     "xcancel",
				Type:     "domain_replace",
				ProxyURL: "https://xcancel.com/",
			},
			targetURL:  "https://x.com/jack/status/20",
			wantPrefix: "https://xcancel.com/jack/status/20",
		},
		{
			name: "youtube shortlink to invidious watch url",
			entry: Entry{
				Service:  "youtube",
				Tech:     "invidious",
				Type:     "domain_replace",
				ProxyURL: "https://inv.nadeko.net/",
			},
			targetURL:  "https://youtu.be/dQw4w9WgXcQ",
			wantPrefix: "https://inv.nadeko.net/watch?v=dQw4w9WgXcQ",
		},
		{
			name: "ens limo append extension",
			entry: Entry{
				Service:  "ens",
				Tech:     "limo",
				Type:     "append_ext",
				ProxyURL: "https://eth.limo/",
			},
			targetURL:  "https://vitalik.eth/posts/1",
			wantPrefix: "https://vitalik.eth.limo/posts/1",
		},
		{
			name: "i2p prepend proxy",
			entry: Entry{
				Service:  "i2p",
				Tech:     "i2p.surf",
				Type:     "prepend",
				ProxyURL: "https://i2p.surf/proxy/",
			},
			targetURL:  "http://forum.i2p/index.php",
			wantPrefix: "https://i2p.surf/proxy/http://forum.i2p/index.php",
		},
		{
			name: "general query param proxy",
			entry: Entry{
				Service:  "general",
				Tech:     "archive.is",
				Type:     "query_param",
				ProxyURL: "https://archive.is/?url=",
			},
			targetURL:  "https://example.com/article",
			wantPrefix: "https://archive.is/?url=",
			wantSubstr: "https%3A%2F%2Fexample.com%2Farticle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transform(tt.entry, tt.targetURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("got %q, want prefix %q", got, tt.wantPrefix)
			}
			if tt.wantSubstr != "" && !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("got %q, want substring %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestPickRandom(t *testing.T) {
	entries := []Entry{
		{Tech: "one"},
		{Tech: "two"},
		{Tech: "three"},
	}

	picked, err := PickRandom(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	valid := false
	for _, e := range entries {
		if e.Tech == picked.Tech {
			valid = true
			break
		}
	}
	if !valid {
		t.Errorf("picked item %v not in candidates list", picked)
	}
}
