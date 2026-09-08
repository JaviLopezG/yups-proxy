package metadata

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Cache manages thread-safe caching of smart card metadata with first-seen tracking.
type Cache struct {
	mu     sync.RWMutex
	items  map[string]*CardData
	ttl    time.Duration
	client *http.Client
}

// NewCache creates an in-memory metadata cache.
func NewCache(ttl time.Duration, client *http.Client) *Cache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if client == nil {
		client = NewSafeHTTPClient(3 * time.Second)
	}
	return &Cache{
		items:  make(map[string]*CardData),
		ttl:    ttl,
		client: client,
	}
}

// Get retrieves cached metadata for a normalized URL.
func (c *Cache) Get(targetURL string) (*CardData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[targetURL]
	if !exists {
		return nil, false
	}

	// Check if entry expired
	if time.Since(item.FetchedAt) > c.ttl {
		return nil, false
	}

	return item, true
}

// Set stores metadata in the cache, preserving the original FirstSeenAt if already present.
func (c *Cache) Set(targetURL string, card *CardData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	if existing, exists := c.items[targetURL]; exists {
		card.FirstSeenAt = existing.FirstSeenAt
	} else if card.FirstSeenAt.IsZero() {
		card.FirstSeenAt = now
	}
	card.FetchedAt = now

	c.items[targetURL] = card
}

// GetOrFetch fetches metadata if not cached, or returns cached data.
// If outbound fetching fails (e.g. timeout or blocked IP), returns a sensible fallback card
// without failing the user request.
func (c *Cache) GetOrFetch(ctx context.Context, targetURL string) *CardData {
	if cached, ok := c.Get(targetURL); ok {
		return cached
	}

	card := c.fetch(ctx, targetURL)
	c.Set(targetURL, card)
	return card
}

func chooseScraperUserAgent(targetHost string) string {
	targetHost = strings.ToLower(strings.TrimPrefix(targetHost, "www."))
	metaDomains := []string{
		"instagram.com",
		"instagr.am",
		"facebook.com",
		"fb.com",
		"fb.watch",
		"whatsapp.com",
		"threads.net",
	}

	for _, domain := range metaDomains {
		if targetHost == domain || strings.HasSuffix(targetHost, "."+domain) {
			return "Twitterbot/1.0"
		}
	}

	return "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)"
}

func (c *Cache) fetch(ctx context.Context, targetURL string) *CardData {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("[YUPS SCRAPER] INVALID URL | %s: %v", targetURL, err)
		return defaultFallbackCard(targetURL, "Unknown")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		log.Printf("[YUPS SCRAPER] REQ ERROR | %s: %v", targetURL, err)
		return defaultFallbackCard(targetURL, parsed.Hostname())
	}

	ua := chooseScraperUserAgent(parsed.Hostname())
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,es;q=0.8")

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start).Round(time.Millisecond)

	if err != nil {
		log.Printf("[YUPS SCRAPER] ERROR (%v) | ua=%s | %s: %v", duration, ua, targetURL, err)
		return defaultFallbackCard(targetURL, parsed.Hostname())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[YUPS SCRAPER] HTTP %d (%v) | ua=%s | %s", resp.StatusCode, duration, ua, targetURL)
		return defaultFallbackCard(targetURL, parsed.Hostname())
	}

	// Read at most 512KB to avoid unbounded memory consumption
	limitedReader := io.LimitReader(resp.Body, 512*1024)
	extracted, err := ExtractFromHTML(limitedReader, targetURL)
	if err != nil {
		log.Printf("[YUPS SCRAPER] PARSE ERROR (%v) | %s: %v", duration, targetURL, err)
		return defaultFallbackCard(targetURL, parsed.Hostname())
	}

	log.Printf("[YUPS SCRAPER] OK 200 (%v) | %s -> title=%q, image=%q",
		duration, targetURL, extracted.Title, extracted.Image)

	return extracted
}

func defaultFallbackCard(targetURL, domain string) *CardData {
	now := time.Now().UTC()
	return &CardData{
		URL:         targetURL,
		Title:       fmt.Sprintf("%s on YUPS", domain),
		Description: fmt.Sprintf("View content from %s using privacy-preserving proxy mirrors.", domain),
		Image:       "/static/logo.png",
		SiteName:    domain,
		FirstSeenAt: now,
		FetchedAt:   now,
	}
}
