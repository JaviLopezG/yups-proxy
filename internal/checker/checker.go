package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/javilopezg/yups-proxy/internal/proxy"
)

const (
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	defaultTargetURL = "https://wikipedia.org"
)

// Default service target URLs for proxy availability checking.
// Wikipedia profiles/pages are used where available, plus specific alternative targets.
var defaultServiceTargets = map[string]string{
	"youtube":   "https://www.youtube.com/wikipedia",
	"instagram": "https://www.instagram.com/wikipedia",
	"twitter":   "https://x.com/wikipedia",
	"goodreads": "https://www.goodreads.com/author/show/16189742.Wikipedia",
	"imgur":     "https://imgur.com/user/Wikipedia",
	"reddit":    "https://www.reddit.com/user/wikipedia/",
	"medium":    "https://ev.medium.com/welcome-to-medium-9e53ca408c48",
	"general":   "https://wikipedia.org",
	"i2p":       "https://stormycloud.i2p/",
	"ens":       "https://zerolend.eth/",
}

// Error substrings to detect in HTTP 200 response bodies.
var errorBodyPatterns = []string{
	"error 4",
	"error 5",
	"<h1>error",
	"<h2>error",
	"<h3>error",
	"<h4>error",
	"<h5>error",
}

var headerErrorRegex = regexp.MustCompile(`(?i)<h[1-5][^>]*>\s*error`)

// CheckResult represents the outcome of probing an individual proxy entry.
type CheckResult struct {
	Index      int    `json:"index"`
	Service    string `json:"service"`
	Tech       string `json:"tech"`
	TestURL    string `json:"test_url"`
	Status     string `json:"status"`
	IsActive   bool   `json:"is_active"`
	IsSkipped  bool   `json:"is_skipped"`
	IsFallback bool   `json:"is_fallback"`
}

// Tag returns the display tag and style class for the check result.
func (r CheckResult) Tag() string {
	if r.IsSkipped {
		return "[SKIP]"
	}
	if r.IsFallback {
		return "[PASS*]"
	}
	if r.IsActive {
		return "[PASS]"
	}
	return "[FAIL]"
}

// TagClass returns the CSS class name for styling the tag.
func (r CheckResult) TagClass() string {
	if r.IsSkipped {
		return "tag-skip"
	}
	if r.IsFallback {
		return "tag-fallback"
	}
	if r.IsActive {
		return "tag-pass"
	}
	return "tag-fail"
}

// ServiceCounts holds active/inactive/skipped statistics for a service.
type ServiceCounts struct {
	Service  string `json:"service"`
	Active   int    `json:"active"`
	Inactive int    `json:"inactive"`
	Skipped  int    `json:"skipped"`
	Fallback int    `json:"fallback"`
}

// Summary holds aggregate results of a health check run.
type Summary struct {
	CheckedAt        time.Time       `json:"checked_at"`
	Duration         time.Duration   `json:"duration"`
	Total            int             `json:"total"`
	Active           int             `json:"active"`
	Inactive         int             `json:"inactive"`
	Skipped          int             `json:"skipped"`
	Fallback         int             `json:"fallback"`
	FallbackServices []string        `json:"fallback_services"`
	ByService        []ServiceCounts `json:"by_service"`
}

// Report encapsulates a complete check cycle with summary and itemized results.
type Report struct {
	Summary Summary       `json:"summary"`
	Results []CheckResult `json:"results"`
}

// Checker manages recurring background proxy availability testing.
type Checker struct {
	registry    *proxy.Registry
	interval    time.Duration
	timeout     time.Duration
	concurrency int
	client      *http.Client
	stopChan    chan struct{}
	startOnce   sync.Once
	stopOnce    sync.Once

	mu         sync.RWMutex
	lastReport *Report
	inProgress bool
}

// New creates a new background proxy Checker.
func New(registry *proxy.Registry, interval, timeout time.Duration, concurrency int) *Checker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if concurrency <= 0 {
		concurrency = 15
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Match python CERT_NONE to allow self-signed proxy certs
		},
		DisableKeepAlives: true,
		MaxIdleConns:      100,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return &Checker{
		registry:    registry,
		interval:    interval,
		timeout:     timeout,
		concurrency: concurrency,
		client:      client,
		stopChan:    make(chan struct{}),
	}
}

// Start launches the recurring background health check worker.
func (c *Checker) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		go func() {
			// Run initial check immediately
			c.RunCheck(ctx)

			ticker := time.NewTicker(c.interval)
			defer ticker.Stop()

			for {
				select {
				case <-c.stopChan:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					c.RunCheck(ctx)
				}
			}
		}()
	})
}

// Stop terminates the background check worker.
func (c *Checker) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})
}

// LastReport returns the most recent health check report, or nil if no check completed yet.
func (c *Checker) LastReport() *Report {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastReport
}

// SetReportForTesting sets the last check report for testing purposes.
func (c *Checker) SetReportForTesting(r *Report) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastReport = r
}

// IsInProgress returns whether a health check cycle is currently running.
func (c *Checker) IsInProgress() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.inProgress
}

// Interval returns the configured interval between checks.
func (c *Checker) Interval() time.Duration {
	return c.interval
}

// BuildTestURL generates the target URL for testing a specific proxy entry.
func BuildTestURL(entry proxy.Entry) string {
	targetURL, ok := defaultServiceTargets[strings.ToLower(strings.TrimSpace(entry.Service))]
	if !ok {
		targetURL = defaultTargetURL
	}
	transformed, err := proxy.Transform(entry, targetURL)
	if err != nil {
		return entry.ProxyURL
	}
	return transformed
}

// FindErrorInBody checks if a lowercase response body contains error indicators.
func FindErrorInBody(bodyLower string) string {
	for _, pattern := range errorBodyPatterns {
		if strings.Contains(bodyLower, pattern) {
			return pattern
		}
	}
	if loc := headerErrorRegex.FindString(bodyLower); loc != "" {
		return strings.TrimSpace(loc)
	}
	return ""
}

// EvaluateHealthyResponse parses HTTP status and headers.
func EvaluateHealthyResponse(statusCode int, header http.Header) (string, bool) {
	if statusCode == 418 {
		return "418 (Teapot)", true
	}

	cfMitigated := header.Get("cf-mitigated")
	if cfMitigated != "" {
		detail := fmt.Sprintf("%d (CF %s)", statusCode, cfMitigated)
		if statusCode == http.StatusOK || statusCode == http.StatusMovedPermanently || statusCode == http.StatusFound || statusCode == http.StatusTemporaryRedirect || statusCode == http.StatusPermanentRedirect {
			detail = strconv.Itoa(statusCode)
		}
		return detail, true
	}

	serverHdr := strings.ToLower(header.Get("server"))
	isCloudflare := strings.Contains(serverHdr, "cloudflare") || header.Get("cf-ray") != ""
	if isCloudflare {
		is5xx := statusCode >= 500 && statusCode <= 599
		is404 := statusCode == http.StatusNotFound
		if !is5xx && !is404 {
			detail := fmt.Sprintf("%d (Cloudflare)", statusCode)
			if statusCode == http.StatusOK || statusCode == http.StatusMovedPermanently || statusCode == http.StatusFound || statusCode == http.StatusTemporaryRedirect || statusCode == http.StatusPermanentRedirect {
				detail = strconv.Itoa(statusCode)
			}
			return detail, true
		}
		if is5xx {
			return fmt.Sprintf("%d (Cloudflare error)", statusCode), false
		}
		return fmt.Sprintf("%d (Not Found)", statusCode), false
	}

	return strconv.Itoa(statusCode), statusCode >= 200 && statusCode < 400
}

func (c *Checker) probeURL(ctx context.Context, targetURL string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "Invalid URL", false
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil || strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "deadline") {
			return "Timeout", false
		}
		return "Connection error", false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// Read up to 512KB to inspect for error strings
		lr := io.LimitReader(resp.Body, 512*1024)
		bodyBytes, err := io.ReadAll(lr)
		if err == nil {
			bodyLower := strings.ToLower(string(bodyBytes))
			if matched := FindErrorInBody(bodyLower); matched != "" {
				return fmt.Sprintf("200 (Error: %s)", matched), false
			}
		}
	}

	return EvaluateHealthyResponse(resp.StatusCode, resp.Header)
}

type probeJob struct {
	index   int
	entry   proxy.Entry
	testURL string
}

type probeResult struct {
	index    int
	testURL  string
	status   string
	isActive bool
}

// RunCheck executes a full health check across all registry entries.
func (c *Checker) RunCheck(ctx context.Context) *Report {
	c.mu.Lock()
	c.inProgress = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.inProgress = false
		c.mu.Unlock()
	}()

	start := time.Now()
	entries := c.registry.Entries()
	total := len(entries)

	resultsMap := make(map[int]CheckResult, total)
	var jobsToProbe []probeJob

	for idx, entry := range entries {
		testURL := BuildTestURL(entry)
		if !entry.AutoCheck {
			// Manual review; retain current active state
			resultsMap[idx] = CheckResult{
				Index:      idx,
				Service:    entry.Service,
				Tech:       entry.Tech,
				TestURL:    testURL,
				Status:     fmt.Sprintf("manual review (kept %s)", formatBoolState(entry.Active)),
				IsActive:   entry.Active,
				IsSkipped:  true,
				IsFallback: false,
			}
		} else {
			jobsToProbe = append(jobsToProbe, probeJob{
				index:   idx,
				entry:   entry,
				testURL: testURL,
			})
		}
	}

	// Worker pool execution
	if len(jobsToProbe) > 0 {
		jobChan := make(chan probeJob, len(jobsToProbe))
		resultChan := make(chan probeResult, len(jobsToProbe))

		numWorkers := c.concurrency
		if numWorkers > len(jobsToProbe) {
			numWorkers = len(jobsToProbe)
		}

		var wg sync.WaitGroup
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobChan {
					status, isActive := c.probeURL(ctx, job.testURL)
					resultChan <- probeResult{
						index:    job.index,
						testURL:  job.testURL,
						status:   status,
						isActive: isActive,
					}
				}
			}()
		}

		for _, job := range jobsToProbe {
			jobChan <- job
		}
		close(jobChan)

		wg.Wait()
		close(resultChan)

		for res := range resultChan {
			entry := entries[res.index]
			resultsMap[res.index] = CheckResult{
				Index:      res.index,
				Service:    entry.Service,
				Tech:       entry.Tech,
				TestURL:    res.testURL,
				Status:     res.status,
				IsActive:   res.isActive,
				IsSkipped:  false,
				IsFallback: false,
			}
		}
	}

	// Evaluate category all-down fallback: if all probed proxies in a service are down,
	// mark them all active as fallback.
	serviceIndices := make(map[string][]int)
	for idx, entry := range entries {
		svc := strings.ToLower(strings.TrimSpace(entry.Service))
		serviceIndices[svc] = append(serviceIndices[svc], idx)
	}

	var fallbackServices []string
	for svc, indices := range serviceIndices {
		var probedIndices []int
		for _, idx := range indices {
			if !resultsMap[idx].IsSkipped {
				probedIndices = append(probedIndices, idx)
			}
		}

		if len(probedIndices) > 0 {
			healthyProbed := 0
			for _, idx := range probedIndices {
				if resultsMap[idx].IsActive {
					healthyProbed++
				}
			}

			if healthyProbed == 0 {
				fallbackServices = append(fallbackServices, svc)
				for _, idx := range probedIndices {
					res := resultsMap[idx]
					res.IsActive = true
					res.IsFallback = true
					res.Status = fmt.Sprintf("%s (all category down -> kept active)", res.Status)
					resultsMap[idx] = res
				}
			}
		}
	}
	sort.Strings(fallbackServices)

	// Update in-memory registry active states
	activeStates := make(map[int]bool, total)
	for idx, res := range resultsMap {
		activeStates[idx] = res.IsActive
	}
	c.registry.UpdateActiveStates(activeStates)

	// Assemble summary statistics
	activeCount := 0
	inactiveCount := 0
	skippedCount := 0
	fallbackCount := 0
	byServiceMap := make(map[string]*ServiceCounts)

	orderedResults := make([]CheckResult, total)
	for idx := 0; idx < total; idx++ {
		res := resultsMap[idx]
		orderedResults[idx] = res

		svc := strings.TrimSpace(res.Service)
		counts, exists := byServiceMap[svc]
		if !exists {
			counts = &ServiceCounts{Service: svc}
			byServiceMap[svc] = counts
		}

		if res.IsSkipped {
			skippedCount++
			counts.Skipped++
			if res.IsActive {
				activeCount++
				counts.Active++
			} else {
				inactiveCount++
				counts.Inactive++
			}
		} else if res.IsFallback {
			fallbackCount++
			activeCount++
			counts.Active++
			counts.Fallback++
		} else {
			if res.IsActive {
				activeCount++
				counts.Active++
			} else {
				inactiveCount++
				counts.Inactive++
			}
		}
	}

	byServiceList := make([]ServiceCounts, 0, len(byServiceMap))
	for _, counts := range byServiceMap {
		byServiceList = append(byServiceList, *counts)
	}
	sort.Slice(byServiceList, func(i, j int) bool {
		return byServiceList[i].Service < byServiceList[j].Service
	})

	report := &Report{
		Summary: Summary{
			CheckedAt:        start,
			Duration:         time.Since(start),
			Total:            total,
			Active:           activeCount,
			Inactive:         inactiveCount,
			Skipped:          skippedCount,
			Fallback:         fallbackCount,
			FallbackServices: fallbackServices,
			ByService:        byServiceList,
		},
		Results: orderedResults,
	}

	c.mu.Lock()
	c.lastReport = report
	c.mu.Unlock()

	return report
}

func formatBoolState(b bool) string {
	if b {
		return "active"
	}
	return "inactive"
}
