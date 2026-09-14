package proxy

import (
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strings"
	"sync"

	"github.com/javilopezg/yups-proxy/data"
)

// Entry represents a single proxy provider configuration.
type Entry struct {
	Service     string
	Tech        string
	Type        string
	ProxyURL    string
	Patterns    []string
	Description string
	Active      bool
	AutoCheck   bool
}

// Registry manages proxy definitions and transformation rules with thread-safe access.
type Registry struct {
	mu      sync.RWMutex
	entries []Entry
}

// NewRegistry creates an empty proxy registry.
func NewRegistry() *Registry {
	return &Registry{entries: make([]Entry, 0)}
}

// LoadFromReader populates the registry from a CSV reader.
func (r *Registry) LoadFromReader(reader io.Reader) error {
	csvReader := csv.NewReader(reader)
	records, err := csvReader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read proxies csv: %w", err)
	}

	if len(records) == 0 {
		return fmt.Errorf("empty proxies csv")
	}

	// First row is header: service,tech,type,proxy_url,patterns,description,active,auto-check
	header := records[0]
	activeColIdx := -1
	autoCheckColIdx := -1
	for colIdx, colName := range header {
		norm := strings.ToLower(strings.TrimSpace(colName))
		norm = strings.ReplaceAll(norm, "_", "-")
		if norm == "active" {
			activeColIdx = colIdx
		} else if norm == "auto-check" || norm == "autocheck" || norm == "check" {
			autoCheckColIdx = colIdx
		}
	}

	entries := make([]Entry, 0, len(records)-1)
	for idx, row := range records[1:] {
		if len(row) < 5 {
			return fmt.Errorf("row %d has insufficient columns (expected at least 5)", idx+2)
		}
		service := strings.TrimSpace(row[0])
		tech := strings.TrimSpace(row[1])
		proxyType := strings.TrimSpace(row[2])
		proxyURL := strings.TrimSpace(row[3])
		rawPatterns := strings.TrimSpace(row[4])
		description := ""
		if len(row) > 5 {
			description = strings.TrimSpace(row[5])
		}

		active := true
		if activeColIdx != -1 && len(row) > activeColIdx {
			active = parseBool(row[activeColIdx], true)
		} else if len(row) > 6 {
			active = parseBool(row[6], true)
		}

		autoCheck := true
		if autoCheckColIdx != -1 && len(row) > autoCheckColIdx {
			autoCheck = parseBool(row[autoCheckColIdx], true)
		} else if len(row) > 7 {
			autoCheck = parseBool(row[7], true)
		}

		patternParts := strings.Split(rawPatterns, ",")
		patterns := make([]string, 0, len(patternParts))
		for _, p := range patternParts {
			p = strings.TrimSpace(p)
			if p != "" {
				patterns = append(patterns, strings.ToLower(p))
			}
		}

		entries = append(entries, Entry{
			Service:     service,
			Tech:        tech,
			Type:        proxyType,
			ProxyURL:    proxyURL,
			Patterns:    patterns,
			Description: description,
			Active:      active,
			AutoCheck:   autoCheck,
		})
	}

	// Automatically register all proxy hostnames as valid patterns for their respective services,
	// without requiring manual duplication in the CSV 'patterns' column.
	// We preserve original patterns in their initial positions so Patterns[0] remains the canonical domain.
	proxyHostsByService := make(map[string][]string)
	for _, entry := range entries {
		if entry.Service == "general" {
			continue
		}
		if pURL, err := url.Parse(entry.ProxyURL); err == nil {
			host := strings.ToLower(pURL.Hostname())
			cleanHost := strings.TrimPrefix(host, "www.")
			if cleanHost != "" {
				hosts := proxyHostsByService[entry.Service]
				found := false
				for _, h := range hosts {
					if h == cleanHost {
						found = true
						break
					}
				}
				if !found {
					proxyHostsByService[entry.Service] = append(hosts, cleanHost)
				}
			}
		}
	}

	for i := range entries {
		if entries[i].Service == "general" {
			continue
		}
		serviceHosts := proxyHostsByService[entries[i].Service]
		for _, sh := range serviceHosts {
			found := false
			for _, pat := range entries[i].Patterns {
				if pat == sh {
					found = true
					break
				}
			}
			if !found {
				entries[i].Patterns = append(entries[i].Patterns, sh)
			}
		}
	}

	r.mu.Lock()
	r.entries = entries
	r.mu.Unlock()
	return nil
}

func parseBool(val string, defaultVal bool) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	switch v {
	case "true", "1", "yes", "y", "t", "active":
		return true
	case "false", "0", "no", "n", "f", "inactive":
		return false
	case "":
		return defaultVal
	default:
		return defaultVal
	}
}

// LoadDefault loads the embedded default proxy dataset.
func (r *Registry) LoadDefault() error {
	return r.LoadFromReader(strings.NewReader(string(data.DefaultProxiesCSV)))
}

// Entries returns a copy of all loaded entries.
func (r *Registry) Entries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copied := make([]Entry, len(r.entries))
	copy(copied, r.entries)
	return copied
}

// ActiveEntries returns all loaded entries that are marked active.
func (r *Registry) ActiveEntries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var active []Entry
	for _, e := range r.entries {
		if e.Active {
			active = append(active, e)
		}
	}
	return active
}

// UpdateActiveStates applies new active statuses to entries by their index in a thread-safe way.
func (r *Registry) UpdateActiveStates(activeStates map[int]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx, active := range activeStates {
		if idx >= 0 && idx < len(r.entries) {
			r.entries[idx].Active = active
		}
	}
}

// NormalizeURL cleans and ensures a valid absolute URL scheme.
func NormalizeURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("empty url")
	}

	// Decode if double-encoded or contains encoded scheme (%3A%2F%2F)
	if strings.Contains(trimmed, "%3A%2F%2F") || strings.Contains(trimmed, "%3a%2f%2f") {
		if unescaped, err := url.QueryUnescape(trimmed); err == nil {
			trimmed = unescaped
		}
	}

	// Add default https scheme if missing
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("missing host in url: %s", trimmed)
	}

	return parsed.String(), nil
}

// MatchService determines which service matches the given normalized URL.
// Returns the service name and the list of available proxy entries for it.
func (r *Registry) MatchService(targetURL string) (string, []Entry) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return "general", r.filterService("general")
	}

	host := strings.ToLower(parsed.Hostname())
	cleanHost := strings.TrimPrefix(host, "www.")

	// 1. Try matching service-specific patterns first
	var matchedService string
	for _, entry := range r.entries {
		if !entry.Active || entry.Service == "general" {
			continue
		}
		for _, pattern := range entry.Patterns {
			if matchesPattern(cleanHost, pattern) {
				matchedService = entry.Service
				break
			}
		}
		if matchedService != "" {
			break
		}
	}

	if matchedService != "" {
		candidates := r.filterService(matchedService)
		if len(candidates) > 0 {
			return matchedService, candidates
		}
	}

	// 2. Fallback to general proxies
	return "general", r.filterService("general")
}

func (r *Registry) filterService(service string) []Entry {
	var results []Entry
	for _, entry := range r.entries {
		if !entry.Active {
			continue
		}
		if strings.EqualFold(entry.Service, service) {
			results = append(results, entry)
		}
	}
	return results
}

// MatchServiceAll determines which service matches the given normalized URL,
// returning the service name and all proxy entries (active, manual, and inactive) for it.
func (r *Registry) MatchServiceAll(targetURL string) (string, []Entry) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return "general", r.filterServiceAll("general")
	}

	host := strings.ToLower(parsed.Hostname())
	cleanHost := strings.TrimPrefix(host, "www.")

	// 1. Try matching service-specific patterns first across all entries
	var matchedService string
	for _, entry := range r.entries {
		if entry.Service == "general" {
			continue
		}
		for _, pattern := range entry.Patterns {
			if matchesPattern(cleanHost, pattern) {
				matchedService = entry.Service
				break
			}
		}
		if matchedService != "" {
			break
		}
	}

	if matchedService != "" {
		candidates := r.filterServiceAll(matchedService)
		if len(candidates) > 0 {
			return matchedService, candidates
		}
	}

	// 2. Fallback to general proxies
	return "general", r.filterServiceAll("general")
}

func (r *Registry) filterServiceAll(service string) []Entry {
	var results []Entry
	for _, entry := range r.entries {
		if strings.EqualFold(entry.Service, service) {
			results = append(results, entry)
		}
	}
	return results
}

func matchesPattern(host, pattern string) bool {
	pattern = strings.ToLower(pattern)
	if pattern == "*" {
		return true
	}
	// Wildcard e.g. *.eth or *.i2p
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(host, suffix)
	}
	// Exact host or subdomain match
	if host == pattern || strings.HasSuffix(host, "."+pattern) {
		return true
	}
	return false
}

// PickRandom selects a random entry from the candidates list.
func PickRandom(candidates []Entry) (Entry, error) {
	if len(candidates) == 0 {
		return Entry{}, fmt.Errorf("no proxy candidates available")
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}

	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
	if err != nil {
		return candidates[0], nil
	}
	return candidates[nBig.Int64()], nil
}

// Transform converts the original target URL into the destination proxy URL.
func Transform(entry Entry, targetURL string) (string, error) {
	parsedTarget, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse target url: %w", err)
	}

	switch entry.Type {
	case "query_param":
		base := entry.ProxyURL
		if strings.HasSuffix(base, "=") {
			return base + url.QueryEscape(targetURL), nil
		}
		if strings.Contains(base, "?") {
			return base + "&url=" + url.QueryEscape(targetURL), nil
		}
		return base + "?url=" + url.QueryEscape(targetURL), nil

	case "prepend":
		base := entry.ProxyURL
		cleanTarget := targetURL
		if idx := strings.Index(cleanTarget, "://"); idx != -1 {
			cleanTarget = cleanTarget[idx+3:]
		}
		if strings.HasSuffix(base, "/") && strings.HasPrefix(cleanTarget, "/") {
			cleanTarget = strings.TrimPrefix(cleanTarget, "/")
		} else if !strings.HasSuffix(base, "/") && !strings.HasPrefix(cleanTarget, "/") {
			base += "/"
		}
		return base + cleanTarget, nil

	case "append_ext":
		proxyParsed, err := url.Parse(entry.ProxyURL)
		if err != nil {
			return "", fmt.Errorf("invalid proxy url: %w", err)
		}
		proxyHost := proxyParsed.Hostname()
		targetHost := parsedTarget.Hostname()

		var newHost string
		if strings.HasPrefix(proxyHost, "eth.") && strings.HasSuffix(targetHost, ".eth") {
			ext := strings.TrimPrefix(proxyHost, "eth.")
			newHost = fmt.Sprintf("%s.%s", targetHost, ext)
		} else {
			newHost = fmt.Sprintf("%s.%s", targetHost, proxyHost)
		}

		result := *parsedTarget
		result.Scheme = proxyParsed.Scheme
		if result.Scheme == "" {
			result.Scheme = "https"
		}
		result.Host = newHost
		return result.String(), nil

	case "domain_replace":
		proxyParsed, err := url.Parse(entry.ProxyURL)
		if err != nil {
			return "", fmt.Errorf("invalid proxy url: %w", err)
		}

		result := *parsedTarget
		result.Scheme = proxyParsed.Scheme
		if result.Scheme == "" {
			result.Scheme = "https"
		}
		result.Host = proxyParsed.Host

		// Special case: YouTube shortlinks (youtu.be/ID -> /watch?v=ID)
		targetHost := strings.ToLower(strings.TrimPrefix(parsedTarget.Hostname(), "www."))
		if targetHost == "youtu.be" {
			videoID := strings.TrimPrefix(parsedTarget.Path, "/")
			if videoID != "" {
				result.Path = "/watch"
				q := result.Query()
				q.Set("v", videoID)
				result.RawQuery = q.Encode()
			}
		} else {
			// If proxy URL has a path prefix, preserve and prepend it
			proxyPath := strings.TrimSuffix(proxyParsed.Path, "/")
			if proxyPath != "" {
				result.Path = proxyPath + "/" + strings.TrimPrefix(parsedTarget.Path, "/")
			}
		}

		return result.String(), nil

	default:
		return "", fmt.Errorf("unsupported proxy type: %s", entry.Type)
	}
}

// FindProxy determines whether targetURL points to one of the configured proxies in the registry,
// regardless of whether the proxy is active or inactive.
func (r *Registry) FindProxy(targetURL string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parsedTarget, err := url.Parse(targetURL)
	if err != nil {
		return Entry{}, false
	}

	targetHost := strings.ToLower(parsedTarget.Hostname())
	cleanTargetHost := strings.TrimPrefix(targetHost, "www.")

	for _, entry := range r.entries {
		if entry.Service == "general" {
			continue
		}
		parsedProxy, err := url.Parse(entry.ProxyURL)
		if err != nil {
			continue
		}
		proxyHost := strings.ToLower(parsedProxy.Hostname())
		cleanProxyHost := strings.TrimPrefix(proxyHost, "www.")

		switch entry.Type {
		case "domain_replace":
			if cleanTargetHost == cleanProxyHost || strings.HasSuffix(cleanTargetHost, "."+cleanProxyHost) {
				// Verify path prefix if proxy URL specifies a subpath
				proxyPath := strings.TrimSuffix(parsedProxy.Path, "/")
				if proxyPath == "" || strings.HasPrefix(parsedTarget.Path, proxyPath) {
					return entry, true
				}
			}
		case "append_ext":
			ext := proxyHost
			if strings.HasPrefix(proxyHost, "eth.") {
				ext = strings.TrimPrefix(proxyHost, "eth.")
			}
			if strings.HasSuffix(cleanTargetHost, "."+ext) || cleanTargetHost == cleanProxyHost {
				return entry, true
			}
		case "prepend":
			if cleanTargetHost == cleanProxyHost {
				proxyPath := strings.TrimSuffix(parsedProxy.Path, "/")
				if proxyPath == "" || strings.HasPrefix(parsedTarget.Path, proxyPath) {
					return entry, true
				}
			}
		case "query_param":
			if cleanTargetHost == cleanProxyHost {
				return entry, true
			}
		}
	}

	return Entry{}, false
}

// Revert converts a URL from a known proxy instance back into its original canonical service URL.
// For domain_replace, it uses entry.Patterns[0] as the canonical domain.
func Revert(entry Entry, proxyTargetURL string) (string, error) {
	if len(entry.Patterns) == 0 {
		return "", fmt.Errorf("no patterns defined for service %s", entry.Service)
	}

	parsedTarget, err := url.Parse(proxyTargetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse proxy target url: %w", err)
	}

	switch entry.Type {
	case "domain_replace":
		firstPattern := entry.Patterns[0]
		if firstPattern == "*" || strings.HasPrefix(firstPattern, "*.") {
			return "", fmt.Errorf("invalid canonical domain pattern %q for service %s", firstPattern, entry.Service)
		}

		result := *parsedTarget
		result.Host = firstPattern
		if result.Scheme == "" {
			result.Scheme = "https"
		}

		// Strip proxy path prefix if present
		parsedProxy, err := url.Parse(entry.ProxyURL)
		if err == nil {
			proxyPath := strings.TrimSuffix(parsedProxy.Path, "/")
			if proxyPath != "" && strings.HasPrefix(result.Path, proxyPath) {
				result.Path = strings.TrimPrefix(result.Path, proxyPath)
				if !strings.HasPrefix(result.Path, "/") {
					result.Path = "/" + result.Path
				}
			}
		}

		return result.String(), nil

	case "append_ext":
		parsedProxy, err := url.Parse(entry.ProxyURL)
		if err != nil {
			return "", fmt.Errorf("failed to parse proxy url: %w", err)
		}
		proxyHost := strings.ToLower(parsedProxy.Hostname())
		ext := proxyHost
		if strings.HasPrefix(proxyHost, "eth.") {
			ext = strings.TrimPrefix(proxyHost, "eth.")
		}
		targetHost := strings.ToLower(parsedTarget.Hostname())
		suffix := "." + ext
		if !strings.HasSuffix(targetHost, suffix) {
			return "", fmt.Errorf("target host %q does not end with proxy extension %q", targetHost, ext)
		}
		origHost := targetHost[:len(targetHost)-len(suffix)]
		result := *parsedTarget
		result.Host = origHost
		if result.Scheme == "" {
			result.Scheme = "https"
		}
		return result.String(), nil

	case "prepend":
		parsedProxy, err := url.Parse(entry.ProxyURL)
		if err != nil {
			return "", fmt.Errorf("failed to parse proxy url: %w", err)
		}
		proxyPath := strings.TrimSuffix(parsedProxy.Path, "/")
		targetPath := parsedTarget.Path
		if !strings.HasPrefix(targetPath, proxyPath) {
			return "", fmt.Errorf("target path %q does not have proxy prefix %q", targetPath, proxyPath)
		}
		remainder := strings.TrimPrefix(targetPath, proxyPath)
		remainder = strings.TrimPrefix(remainder, "/")
		if parsedTarget.RawQuery != "" {
			remainder += "?" + parsedTarget.RawQuery
		}
		return NormalizeURL(remainder)

	case "query_param":
		qURL := parsedTarget.Query().Get("url")
		if qURL != "" {
			return NormalizeURL(qURL)
		}
		return "", fmt.Errorf("query parameter 'url' not found in proxy url %s", proxyTargetURL)

	default:
		return "", fmt.Errorf("unsupported proxy type for reversion: %s", entry.Type)
	}
}

// IsSameProxy checks if two entries represent the same proxy service instance.
func IsSameProxy(a, b Entry) bool {
	if strings.EqualFold(strings.TrimRight(a.ProxyURL, "/"), strings.TrimRight(b.ProxyURL, "/")) {
		return true
	}
	pa, errA := url.Parse(a.ProxyURL)
	pb, errB := url.Parse(b.ProxyURL)
	if errA == nil && errB == nil {
		hostA := strings.TrimPrefix(strings.ToLower(pa.Hostname()), "www.")
		hostB := strings.TrimPrefix(strings.ToLower(pb.Hostname()), "www.")
		pathA := strings.TrimSuffix(pa.Path, "/")
		pathB := strings.TrimSuffix(pb.Path, "/")
		if hostA == hostB && pathA == pathB {
			return true
		}
	}
	return false
}
