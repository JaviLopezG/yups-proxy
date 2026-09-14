package checker

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrintReport(t *testing.T) {
	rep := &Report{
		Summary: Summary{
			CheckedAt:        time.Now(),
			Duration:         1200 * time.Millisecond,
			Total:            4,
			Active:           2,
			Inactive:         1,
			Skipped:          1,
			Fallback:         1,
			FallbackServices: []string{"twitter"},
			ByService: []ServiceCounts{
				{Service: "twitter", Active: 1, Inactive: 0, Skipped: 0, Fallback: 1},
				{Service: "scribe", Active: 0, Inactive: 1, Skipped: 1, Fallback: 0},
			},
		},
		Results: []CheckResult{
			{
				Index:      0,
				Service:    "twitter",
				Tech:       "nitter",
				TestURL:    "https://nitter.example.com/Wikipedia",
				Status:     "200 (all category down -> kept active)",
				IsActive:   true,
				IsFallback: true,
			},
			{
				Index:     1,
				Service:   "scribe",
				Tech:      "scribe",
				TestURL:   "https://scribe.example.com",
				Status:    "manual review (kept false)",
				IsActive:  false,
				IsSkipped: true,
			},
		},
	}

	var buf bytes.Buffer
	PrintReport(&buf, rep)
	out := buf.String()

	if !strings.Contains(out, "Proxy Health Summary") {
		t.Errorf("expected summary in output")
	}
	if !strings.Contains(out, "PASS*") {
		t.Errorf("expected PASS* tag in output")
	}
	if !strings.Contains(out, "SKIP") {
		t.Errorf("expected SKIP tag in output")
	}
	if !strings.Contains(out, "Breakdown by service:") {
		t.Errorf("expected breakdown by service in output")
	}
	if !strings.Contains(out, "All probed proxies were down for category: twitter.") {
		t.Errorf("expected fallback notice in output")
	}
}

func TestUpdateCSV(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "proxies.csv")

	initialCSV := "service,tech,type,proxy_url,patterns,description,active,auto-check\n" +
		"twitter,nitter,domain_replace,https://nitter1.com/,*,desc,false,true\n" +
		"twitter,nitter,domain_replace,https://nitter2.com/,*,desc,true,false\n"

	if err := os.WriteFile(csvPath, []byte(initialCSV), 0644); err != nil {
		t.Fatalf("failed to write temp csv: %v", err)
	}

	rep := &Report{
		Results: []CheckResult{
			{
				Index:     0,
				IsActive:  true,
				IsSkipped: false,
			},
			{
				Index:     1,
				IsActive:  false,
				IsSkipped: true,
			},
		},
	}

	if err := UpdateCSV(csvPath, rep); err != nil {
		t.Fatalf("UpdateCSV failed: %v", err)
	}

	updated, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("failed to read updated csv: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(updated)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// First row was false, now active -> should end with true,true
	if !strings.HasSuffix(lines[1], "true,true") {
		t.Errorf("expected line 1 to end with true,true, got %q", lines[1])
	}
	// Second row was active=true, auto-check=false -> now active=false, auto-check=false
	if !strings.HasSuffix(lines[2], "false,false") {
		t.Errorf("expected line 2 to end with false,false, got %q", lines[2])
	}
}
