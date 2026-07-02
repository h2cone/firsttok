package report

import (
	"fmt"
	"strconv"
	"strings"
)

// ReportData carries everything needed to render report.txt.
type ReportData struct {
	IsCompare       bool
	GeneratedAt     string
	OutputDir       string
	Method          string
	Targets         []Target
	ConfigProfiles  []ConfigProfileRow
	EndpointSummary []SummaryRow
	RoundSummary    []SummaryRow
	DeltaSummary    []DeltaSummaryRow
	Failures        []RunRecord
	GeneratedFiles  []string
}

// Render produces the report.txt content.
func (d *ReportData) Render() string {
	var b strings.Builder
	if d.IsCompare {
		d.renderCompare(&b)
	} else {
		d.renderBench(&b)
	}
	return b.String()
}

func (d *ReportData) renderCompare(b *strings.Builder) {
	fmt.Fprintf(b, "TTFT comparison report\n")
	fmt.Fprintf(b, "GeneratedAt: %s\n", d.GeneratedAt)
	fmt.Fprintf(b, "OutputDir: %s\n", d.OutputDir)
	fmt.Fprintf(b, "Method: %s\n", d.Method)
	b.WriteString("\nTargets:\n")
	for _, t := range d.Targets {
		if t.Chain != "" {
			fmt.Fprintf(b, "  %s: %s (%s)\n", t.Key, t.Config, t.Chain)
		} else {
			fmt.Fprintf(b, "  %s: %s\n", t.Key, t.Config)
		}
	}
	b.WriteString("\nConfig profile:\n")
	b.WriteString(renderTable(configProfileTextCols(), configProfileTextRows(d.ConfigProfiles)))
	b.WriteString("\nEndpoint summary, warmups excluded:\n")
	b.WriteString(renderTable(endpointSummaryTextCols(), summaryTextRows(d.EndpointSummary, false)))
	b.WriteString("\nDelta summary, left minus right. Negative means left was faster:\n")
	b.WriteString(renderTable(deltaSummaryTextCols(), deltaSummaryTextRows(d.DeltaSummary)))
	fmt.Fprintf(b, "\nFailures: %d\n", len(d.Failures))
	if len(d.Failures) > 0 {
		b.WriteString(renderTable(failureTextCols(true), failureTextRows(d.Failures, true)))
	}
	b.WriteString("\nGenerated files:\n")
	for _, f := range d.GeneratedFiles {
		fmt.Fprintf(b, "  %s\n", f)
	}
}

func (d *ReportData) renderBench(b *strings.Builder) {
	fmt.Fprintf(b, "TTFT single config report\n")
	fmt.Fprintf(b, "GeneratedAt: %s\n", d.GeneratedAt)
	fmt.Fprintf(b, "OutputDir: %s\n", d.OutputDir)
	fmt.Fprintf(b, "Method: %s\n", d.Method)
	b.WriteString("\nConfig profile:\n")
	b.WriteString(renderTable(configProfileTextCols(), configProfileTextRows(d.ConfigProfiles)))
	b.WriteString("\nSummary, warmups excluded:\n")
	b.WriteString(renderTable(endpointSummaryTextCols(), summaryTextRows(d.EndpointSummary, false)))
	b.WriteString("\nRound summary, warmups excluded:\n")
	b.WriteString(renderTable(roundSummaryTextCols(), summaryTextRows(d.RoundSummary, true)))
	fmt.Fprintf(b, "\nFailures: %d\n", len(d.Failures))
	if len(d.Failures) > 0 {
		b.WriteString(renderTable(failureTextCols(false), failureTextRows(d.Failures, false)))
	}
	b.WriteString("\nGenerated files:\n")
	for _, f := range d.GeneratedFiles {
		fmt.Fprintf(b, "  %s\n", f)
	}
}

// renderTable renders a header + divider + rows table with two-space column
// separators and left-aligned cells.
func renderTable(cols []string, rows [][]string) string {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, r := range rows {
		for i := 0; i < len(cols) && i < len(r); i++ {
			if len(r[i]) > widths[i] {
				widths[i] = len(r[i])
			}
		}
	}
	var b strings.Builder
	b.WriteString("  ")
	for i, c := range cols {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(padRight(c, widths[i]))
	}
	b.WriteString("\n  ")
	for i := range cols {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("-", widths[i]))
	}
	b.WriteString("\n")
	for _, r := range rows {
		b.WriteString("  ")
		for i := range cols {
			if i > 0 {
				b.WriteString("  ")
			}
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			b.WriteString(padRight(cell, widths[i]))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// ---- text column definitions (snake_case per the plan's report.txt spec) ----

func configProfileTextCols() []string {
	return []string{"endpoint", "provider", "api", "base_url_host", "path", "model", "stream", "max_tokens", "request_sha256", "config_path"}
}

func configProfileTextRows(rows []ConfigProfileRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.Endpoint, r.Provider, r.API, r.BaseURLHost, r.Path, r.Model,
			formatBool(r.Stream, FormatDefault), formatMaxTokens(r.MaxTokens),
			r.RequestSHA256, r.ConfigPath,
		})
	}
	return out
}

func endpointSummaryTextCols() []string {
	return []string{"endpoint", "runs", "successful_runs", "failed_runs", "failure_rate_pct",
		"ttft_min_ms", "ttft_p50_ms", "ttft_p90_ms", "ttft_p95_ms", "ttft_p99_ms",
		"ttft_max_ms", "ttft_avg_ms", "ttft_stddev_ms", "ttfb_p50_ms", "headers_p50_ms"}
}

func roundSummaryTextCols() []string {
	return []string{"round", "runs", "successful_runs", "failed_runs", "failure_rate_pct",
		"ttft_p50_ms", "ttft_p90_ms", "ttft_p95_ms", "ttft_avg_ms", "ttft_max_ms"}
}

func summaryTextRows(rows []SummaryRow, isRound bool) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		round := ""
		if r.Round != nil {
			round = strconv.Itoa(*r.Round)
		}
		if isRound {
			out = append(out, []string{
				round, strconv.Itoa(r.Runs), strconv.Itoa(r.SuccessfulRuns),
				strconv.Itoa(r.FailedRuns), formatMS(r.FailureRatePct),
				formatMS(r.TTFTP50), formatMS(r.TTFTP90), formatMS(r.TTFTP95),
				formatMS(r.TTFTAvg), formatMS(r.TTFTMax),
			})
			continue
		}
		out = append(out, []string{
			r.Endpoint, strconv.Itoa(r.Runs), strconv.Itoa(r.SuccessfulRuns),
			strconv.Itoa(r.FailedRuns), formatMS(r.FailureRatePct),
			formatMS(r.TTFTMin), formatMS(r.TTFTP50), formatMS(r.TTFTP90),
			formatMS(r.TTFTP95), formatMS(r.TTFTP99), formatMS(r.TTFTMax),
			formatMS(r.TTFTAvg), formatMS(r.TTFTStdDev), formatMS(r.TTFBP50),
			formatMS(r.HeadersP50),
		})
	}
	return out
}

func deltaSummaryTextCols() []string {
	return DeltaSummaryFields
}

func deltaSummaryTextRows(rows []DeltaSummaryRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.Comparison, r.Meaning, strconv.Itoa(r.RoundsCompared),
			formatMS(r.PooledP50Delta), formatMS(r.PooledP95Delta), formatMS(r.PooledAvgDelta),
			formatMS(r.RoundP50DeltaMedian), formatMS(r.RoundP50DeltaMin), formatMS(r.RoundP50DeltaMax),
			formatMS(r.RoundP95DeltaMedian), formatMS(r.RoundAvgDeltaMedian),
		})
	}
	return out
}

func failureTextCols(withEndpoint bool) []string {
	if withEndpoint {
		return []string{"endpoint", "round", "run", "status", "error"}
	}
	return []string{"round", "run", "status", "error"}
}

func failureTextRows(failures []RunRecord, withEndpoint bool) [][]string {
	out := make([][]string, 0, len(failures))
	for _, r := range failures {
		round := ""
		if r.HasRound {
			round = strconv.Itoa(r.Round)
		}
		status := ""
		if r.Status != 0 {
			status = strconv.Itoa(r.Status)
		}
		if withEndpoint {
			out = append(out, []string{r.Endpoint, round, strconv.Itoa(r.Run), status, r.Error})
		} else {
			out = append(out, []string{round, strconv.Itoa(r.Run), status, r.Error})
		}
	}
	return out
}
