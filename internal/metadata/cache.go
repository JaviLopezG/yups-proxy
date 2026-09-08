package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func (c *Cache) fetch(ctx context.Context, targetURL string) *CardData {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return defaultFallbackCard(targetURL, "Unknown")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return defaultFallbackCard(targetURL, parsed.Hostname())
	}

	// Realistic browser UA to obtain OG tags without getting 403
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; YupsBot/1.0; +https://yups.io)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.client.Do(req)
	if err != nil {
		return defaultFallbackCard(targetURL, parsed.Hostname())
	}
	defer resp.Body.Close()

	// Read at most 512KB to avoid unbounded memory consumption
	limitedReader := io.LimitReader(resp.Body, 512*1024)
	extracted, err := ExtractFromHTML(limitedReader, targetURL)
	if err != nil {
		return defaultFallbackCard(targetURL, parsed.Hostname())
	}

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
