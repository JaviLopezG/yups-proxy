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
			name:    "empty url errors",
			input:   "   ",
			wantErr: true,
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
			wantPrefix: "https://i2p.surf/proxy/forum.i2p/index.php",
		},
		{
			name: "i2p prepend proxy with https",
			entry: Entry{
				Service:  "i2p",
				Tech:     "i2p.surf",
				Type:     "prepend",
				ProxyURL: "https://i2p.surf/proxy/",
			},
			targetURL:  "https://stormycloud.i2p/",
			wantPrefix: "https://i2p.surf/proxy/stormycloud.i2p/",
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

func TestLoadFromReaderWithActive(t *testing.T) {
	csvData := `service,tech,type,proxy_url,patterns,description,active
general,archive.is,query_param,https://archive.is/submit/?url=,*,Archive.is,true
twitter,nitter1,domain_replace,https://nitter1.example.com/,x.com,Nitter Active,true
twitter,nitter2,domain_replace,https://nitter2.example.com/,x.com,Nitter Inactive,false
reddit,redlib1,domain_replace,https://redlib1.example.com/,reddit.com,Redlib 1,1
reddit,redlib2,domain_replace,https://redlib2.example.com/,reddit.com,Redlib 0,0
`
	reg := NewRegistry()
	if err := reg.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("unexpected error loading csv: %v", err)
	}

	if len(reg.Entries()) != 5 {
		t.Fatalf("expected 5 total entries, got %d", len(reg.Entries()))
	}
	if len(reg.ActiveEntries()) != 3 {
		t.Fatalf("expected 3 active entries, got %d", len(reg.ActiveEntries()))
	}

	// Backward compatibility: CSV without active column should default to true
	legacyCSV := `service,tech,type,proxy_url,patterns,description
general,archive.is,query_param,https://archive.is/submit/?url=,*,Archive.is
twitter,nitter,domain_replace,https://nitter.example.com/,x.com,Nitter
`
	legacyReg := NewRegistry()
	if err := legacyReg.LoadFromReader(strings.NewReader(legacyCSV)); err != nil {
		t.Fatalf("unexpected error loading legacy csv: %v", err)
	}
	if len(legacyReg.ActiveEntries()) != 2 {
		t.Fatalf("expected all legacy entries to be active by default, got %d", len(legacyReg.ActiveEntries()))
	}
}

func TestActiveFiltering(t *testing.T) {
	csvData := `service,tech,type,proxy_url,patterns,description,active
general,archive.is,query_param,https://archive.is/submit/?url=,*,Archive.is,true
twitter,nitter_active,domain_replace,https://active.nitter.example.com/,x.com,Nitter Active,true
twitter,nitter_dead,domain_replace,https://dead.nitter.example.com/,x.com,Nitter Dead,false
reddit,redlib_dead,domain_replace,https://dead.redlib.example.com/,reddit.com,Redlib Dead,false
`
	reg := NewRegistry()
	if err := reg.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Twitter has one active and one inactive mirror
	svc, candidates := reg.MatchService("https://x.com/jack/status/1")
	if svc != "twitter" {
		t.Errorf("expected matched service 'twitter', got %q", svc)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected exactly 1 active candidate for twitter, got %d", len(candidates))
	}
	if candidates[0].Tech != "nitter_active" {
		t.Errorf("expected candidate 'nitter_active', got %q", candidates[0].Tech)
	}

	// Reddit has only inactive mirrors -> should fallback to general
	svc, candidates = reg.MatchService("https://reddit.com/r/golang")
	if svc != "general" {
		t.Errorf("expected fallback service 'general' when all reddit proxies are inactive, got %q", svc)
	}
	if len(candidates) != 1 || candidates[0].Service != "general" {
		t.Fatalf("expected general proxy candidate fallback, got %v", candidates)
	}
}

func TestLoadFromReaderWithAutoCheck(t *testing.T) {
	csvData := `service,tech,type,proxy_url,patterns,description,active,auto-check
general,archive.is,query_param,https://archive.is/submit/?url=,*,Archive.is,true,true
twitter,nitter,domain_replace,https://nitter.net/,x.com,Official Nitter,false,false
instagram,kittygram,domain_replace,https://kittygram.pussthecat.org/,instagram.com,Kittygram,true,false
`
	reg := NewRegistry()
	if err := reg.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("unexpected error loading csv: %v", err)
	}

	entries := reg.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if !entries[0].Active || !entries[0].AutoCheck {
		t.Errorf("expected archive.is to be active and auto-check true, got active=%v, autocheck=%v", entries[0].Active, entries[0].AutoCheck)
	}
	if entries[1].Active || entries[1].AutoCheck {
		t.Errorf("expected nitter.net to be active=false and auto-check=false, got active=%v, autocheck=%v", entries[1].Active, entries[1].AutoCheck)
	}
	if !entries[2].Active || entries[2].AutoCheck {
		t.Errorf("expected pussthecat to be active=true and auto-check=false, got active=%v, autocheck=%v", entries[2].Active, entries[2].AutoCheck)
	}
}

func TestMatchServiceAll(t *testing.T) {
	csvData := `service,tech,type,proxy_url,patterns,description,active,auto-check
twitter,active_nitter,domain_replace,https://active.example.com/,x.com,Active,true,true
twitter,manual_nitter,domain_replace,https://manual.example.com/,x.com,Manual,true,false
twitter,dead_nitter,domain_replace,https://dead.example.com/,x.com,Dead,false,true
`
	reg := NewRegistry()
	if err := reg.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("unexpected error loading csv: %v", err)
	}

	svc, candidates := reg.MatchServiceAll("https://x.com/Wikipedia")
	if svc != "twitter" {
		t.Fatalf("expected 'twitter', got %q", svc)
	}
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates (active, manual, dead), got %d", len(candidates))
	}
}
