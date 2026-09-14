package checker

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[0;33m"
	colorBold   = "\033[1m"
)

// PrintReport writes the formatted health check report with ANSI colors to w.
func PrintReport(w io.Writer, rep *Report) {
	if rep == nil {
		return
	}

	for _, res := range rep.Results {
		tag := coloredTag(res)
		fmt.Fprintf(w, "  %s %-12s %-16s %-40s -> %s\n",
			tag,
			res.Service,
			res.Tech,
			res.TestURL,
			res.Status,
		)
	}

	s := rep.Summary
	fmt.Fprintf(w, "\n%s══════════════════ Proxy Health Summary ══════════════════%s\n", colorBold, colorReset)
	fbSummary := ""
	if s.Fallback > 0 {
		fbSummary = fmt.Sprintf(" | %sCategory Fallback: %d%s", colorYellow, s.Fallback, colorReset)
	}
	fmt.Fprintf(w, "Total: %d | %sActive: %d%s | %sInactive: %d%s | %sManual/Skipped: %d%s%s\n",
		s.Total,
		colorGreen, s.Active, colorReset,
		colorRed, s.Inactive, colorReset,
		colorYellow, s.Skipped, colorReset,
		fbSummary,
	)

	if len(s.FallbackServices) > 0 {
		fmt.Fprintf(w, "\n%s%sNotice:%s All probed proxies were down for category: %s.\n",
			colorYellow, colorBold, colorReset, strings.Join(s.FallbackServices, ", "))
		fmt.Fprintf(w, "They were kept active in case an upstream bot challenge or software update expelled bots.\n\n")
	}

	fmt.Fprintf(w, "%sBreakdown by service:%s\n", colorBold, colorReset)
	for _, counts := range s.ByService {
		color := colorGreen
		if counts.Active == 0 {
			color = colorRed
		}
		skpStr := ""
		if counts.Skipped > 0 {
			skpStr = fmt.Sprintf(", %s%d manual%s", colorYellow, counts.Skipped, colorReset)
		}
		fbStr := ""
		if counts.Fallback > 0 {
			fbStr = fmt.Sprintf(", %s%d fallback%s", colorYellow, counts.Fallback, colorReset)
		}
		fmt.Fprintf(w, "  - %-14s: %s%d active%s, %d inactive%s%s\n",
			counts.Service,
			color, counts.Active, colorReset,
			counts.Inactive,
			skpStr,
			fbStr,
		)
	}
	fmt.Fprintf(w, "%s══════════════════════════════════════════════════════════%s\n", colorBold, colorReset)
}

func coloredTag(res CheckResult) string {
	if res.IsSkipped {
		return fmt.Sprintf("[%sSKIP%s]", colorYellow, colorReset)
	}
	if res.IsFallback {
		return fmt.Sprintf("[%sPASS*%s]", colorYellow, colorReset)
	}
	if res.IsActive {
		return fmt.Sprintf("[%sPASS%s]", colorGreen, colorReset)
	}
	return fmt.Sprintf("[%sFAIL%s]", colorRed, colorReset)
}

// UpdateCSV updates active and auto-check columns in csvPath with results from report.
func UpdateCSV(csvPath string, rep *Report) error {
	f, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("failed to open csv %q: %w", csvPath, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read csv %q: %w", csvPath, err)
	}
	if len(records) == 0 {
		return fmt.Errorf("csv file %q is empty", csvPath)
	}

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

	if activeColIdx == -1 {
		activeColIdx = len(header)
		header = append(header, "active")
	}
	if autoCheckColIdx == -1 {
		autoCheckColIdx = len(header)
		header = append(header, "auto-check")
	}

	resByIndex := make(map[int]CheckResult, len(rep.Results))
	for _, res := range rep.Results {
		resByIndex[res.Index] = res
	}

	updatedRecords := make([][]string, 0, len(records))
	updatedRecords = append(updatedRecords, header)

	for rowIdx, row := range records[1:] {
		for len(row) <= activeColIdx || len(row) <= autoCheckColIdx {
			row = append(row, "")
		}

		if res, exists := resByIndex[rowIdx]; exists {
			if res.IsActive {
				row[activeColIdx] = "true"
			} else {
				row[activeColIdx] = "false"
			}

			if strings.TrimSpace(row[autoCheckColIdx]) == "" {
				if res.IsSkipped {
					row[autoCheckColIdx] = "false"
				} else {
					row[autoCheckColIdx] = "true"
				}
			}
		}
		updatedRecords = append(updatedRecords, row)
	}

	outFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to write csv %q: %w", csvPath, err)
	}
	defer outFile.Close()

	writer := csv.NewWriter(outFile)
	if err := writer.WriteAll(updatedRecords); err != nil {
		return fmt.Errorf("failed to write records to %q: %w", csvPath, err)
	}
	return nil
}
