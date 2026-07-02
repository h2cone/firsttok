package report

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/firsttok/firsttok/internal/config"
)

// Format selects the CSV field contract.
type Format int

const (
	FormatDefault Format = iota
	FormatPerftest
)

// WriteCSV writes rows to path with the given header. When quoteAll is true,
// every field is double-quoted (perftest compat); otherwise Go's minimal
// quoting is used (firsttok default). Line terminator is always "\n".
func WriteCSV(path string, header []string, rows []map[string]string, quoteAll bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if quoteAll {
		bw := bufio.NewWriter(f)
		if err := writeQuoteAll(bw, header, rows); err != nil {
			return err
		}
		return bw.Flush()
	}

	w := csv.NewWriter(f)
	w.UseCRLF = false
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		line := make([]string, len(header))
		for i, h := range header {
			line[i] = r[h]
		}
		if err := w.Write(line); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// writeQuoteAll emits every field double-quoted with LF terminators.
func writeQuoteAll(w *bufio.Writer, header []string, rows []map[string]string) error {
	writeLine := func(fields []string) error {
		for i, v := range fields {
			if i > 0 {
				if err := w.WriteByte(','); err != nil {
					return err
				}
			}
			if err := w.WriteByte('"'); err != nil {
				return err
			}
			if _, err := w.WriteString(strings.ReplaceAll(v, `"`, `""`)); err != nil {
				return err
			}
			if err := w.WriteByte('"'); err != nil {
				return err
			}
		}
		return w.WriteByte('\n')
	}
	if err := writeLine(header); err != nil {
		return err
	}
	for _, r := range rows {
		line := make([]string, len(header))
		for i, h := range header {
			line[i] = r[h]
		}
		if err := writeLine(line); err != nil {
			return err
		}
	}
	return nil
}

// ---- value formatters ----

func formatMS(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 3, 64)
}

func formatInt(v int) string { return strconv.Itoa(v) }

func formatIntPtr(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

func formatBool(b *bool, format Format) string {
	if b == nil {
		return ""
	}
	if format == FormatPerftest {
		if *b {
			return "True"
		}
		return "False"
	}
	if *b {
		return "true"
	}
	return "false"
}

func formatMaxTokens(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// ---- all_runs / failures rows ----

// AllRunsRow renders one run record to a CSV row map for the given format.
func AllRunsRow(r RunRecord, format Format) map[string]string {
	round := ""
	if r.HasRound {
		round = formatInt(r.Round)
	}
	firstTokenSource, firstTokenPreview := "", ""
	if r.FirstToken != nil {
		firstTokenSource = r.FirstToken.Source
		firstTokenPreview = r.FirstToken.Preview
	}
	status := ""
	if r.Status != 0 {
		status = formatInt(r.Status)
	}
	warmup := formatBool(&r.Warmup, format)
	ok := formatBool(&r.OK, format)
	if format == FormatPerftest {
		return map[string]string{
			"Endpoint": r.Endpoint, "Label": r.Label, "Round": round, "Run": formatInt(r.Run),
			"Warmup": warmup, "Ok": ok, "Status": status,
			"TtfbMs": formatMS(r.TTFBMS), "HeadersMs": formatMS(r.HeadersMS),
			"FirstEventMs": formatMS(r.FirstEventMS), "TtftMs": formatMS(r.TTFTMS),
			"EventsRead": formatInt(r.EventsRead), "BytesRead": strconv.FormatInt(r.BytesRead, 10),
			"FirstTokenSource": firstTokenSource, "FirstTokenPreview": firstTokenPreview,
			"Provider": r.Provider, "Url": r.URL, "StartedAt": r.StartedAt,
			"Error": r.Error, "SourceFile": r.SourceFile,
		}
	}
	return map[string]string{
		"endpoint": r.Endpoint, "label": r.Label, "round": round, "run": formatInt(r.Run),
		"warmup": warmup, "ok": ok, "status": status,
		"ttfb_ms": formatMS(r.TTFBMS), "headers_ms": formatMS(r.HeadersMS),
		"first_event_ms": formatMS(r.FirstEventMS), "ttft_ms": formatMS(r.TTFTMS),
		"events_read": formatInt(r.EventsRead), "bytes_read": strconv.FormatInt(r.BytesRead, 10),
		"first_token_source": firstTokenSource, "first_token_preview": firstTokenPreview,
		"provider": r.Provider, "api": r.API, "url": r.URL, "started_at": r.StartedAt,
		"error": r.Error, "source_file": r.SourceFile,
	}
}

// AllRunsMaps renders all run records (including warmup) to row maps.
func AllRunsMaps(runs []RunRecord, format Format) []map[string]string {
	out := make([]map[string]string, 0, len(runs))
	for _, r := range runs {
		out = append(out, AllRunsRow(r, format))
	}
	return out
}

// FailuresMaps renders non-warmup failed run records to row maps.
func FailuresMaps(runs []RunRecord, format Format) []map[string]string {
	var out []map[string]string
	for _, r := range runs {
		if r.Warmup {
			continue
		}
		if r.OK {
			continue
		}
		out = append(out, AllRunsRow(r, format))
	}
	return out
}

// ---- summary rows ----

// SummaryMaps renders summary rows for the given format.
func SummaryMaps(rows []SummaryRow, format Format) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, summaryMap(r, format))
	}
	return out
}

func summaryMap(r SummaryRow, format Format) map[string]string {
	round := ""
	if r.Round != nil {
		round = formatInt(*r.Round)
	}
	if format == FormatPerftest {
		return map[string]string{
			"Endpoint": r.Endpoint, "Label": r.Label, "Round": round,
			"RoundsIncluded": formatInt(r.RoundsIncluded), "Runs": formatInt(r.Runs),
			"SuccessfulRuns": formatInt(r.SuccessfulRuns), "FailedRuns": formatInt(r.FailedRuns),
			"FailureRatePct": formatMS(r.FailureRatePct),
			"TtftMinMs":      formatMS(r.TTFTMin), "TtftP25Ms": formatMS(r.TTFTP25),
			"TtftP50Ms": formatMS(r.TTFTP50), "TtftP75Ms": formatMS(r.TTFTP75),
			"TtftP90Ms": formatMS(r.TTFTP90), "TtftP95Ms": formatMS(r.TTFTP95),
			"TtftP99Ms": formatMS(r.TTFTP99), "TtftMaxMs": formatMS(r.TTFTMax),
			"TtftAvgMs": formatMS(r.TTFTAvg), "TtftStdDevMs": formatMS(r.TTFTStdDev),
			"TtfbP50Ms": formatMS(r.TTFBP50), "HeadersP50Ms": formatMS(r.HeadersP50),
			"FirstEventP50Ms": formatMS(r.FirstEventP50),
			"EventsReadP50":   formatMS(r.EventsReadP50), "BytesReadP50": formatMS(r.BytesReadP50),
		}
	}
	return map[string]string{
		"endpoint": r.Endpoint, "label": r.Label, "round": round,
		"rounds_included": formatInt(r.RoundsIncluded), "runs": formatInt(r.Runs),
		"successful_runs": formatInt(r.SuccessfulRuns), "failed_runs": formatInt(r.FailedRuns),
		"failure_rate_pct": formatMS(r.FailureRatePct),
		"ttft_min_ms":      formatMS(r.TTFTMin), "ttft_p25_ms": formatMS(r.TTFTP25),
		"ttft_p50_ms": formatMS(r.TTFTP50), "ttft_p75_ms": formatMS(r.TTFTP75),
		"ttft_p90_ms": formatMS(r.TTFTP90), "ttft_p95_ms": formatMS(r.TTFTP95),
		"ttft_p99_ms": formatMS(r.TTFTP99), "ttft_max_ms": formatMS(r.TTFTMax),
		"ttft_avg_ms": formatMS(r.TTFTAvg), "ttft_stddev_ms": formatMS(r.TTFTStdDev),
		"ttfb_p50_ms": formatMS(r.TTFBP50), "headers_p50_ms": formatMS(r.HeadersP50),
		"first_event_p50_ms": formatMS(r.FirstEventP50),
		"events_read_p50":    formatMS(r.EventsReadP50), "bytes_read_p50": formatMS(r.BytesReadP50),
	}
}

// ---- delta rows ----

func DeltaByRoundMaps(rows []DeltaRow, format Format) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, deltaByRoundMap(r, format))
	}
	return out
}

func deltaByRoundMap(r DeltaRow, format Format) map[string]string {
	round := ""
	if r.Round != nil {
		round = formatInt(*r.Round)
	}
	if format == FormatPerftest {
		return map[string]string{
			"Round": round, "Comparison": r.Comparison, "Meaning": r.Meaning,
			"LeftEndpoint": r.LeftEndpoint, "RightEndpoint": r.RightEndpoint,
			"P50DeltaMs": formatMS(r.P50Delta), "P90DeltaMs": formatMS(r.P90Delta),
			"P95DeltaMs": formatMS(r.P95Delta), "AvgDeltaMs": formatMS(r.AvgDelta),
			"MinDeltaMs": formatMS(r.MinDelta), "MaxDeltaMs": formatMS(r.MaxDelta),
		}
	}
	return map[string]string{
		"round": round, "comparison": r.Comparison, "meaning": r.Meaning,
		"left_endpoint": r.LeftEndpoint, "right_endpoint": r.RightEndpoint,
		"p50_delta_ms": formatMS(r.P50Delta), "p90_delta_ms": formatMS(r.P90Delta),
		"p95_delta_ms": formatMS(r.P95Delta), "avg_delta_ms": formatMS(r.AvgDelta),
		"min_delta_ms": formatMS(r.MinDelta), "max_delta_ms": formatMS(r.MaxDelta),
	}
}

func DeltaSummaryMaps(rows []DeltaSummaryRow, format Format) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, deltaSummaryMap(r, format))
	}
	return out
}

func deltaSummaryMap(r DeltaSummaryRow, format Format) map[string]string {
	if format == FormatPerftest {
		return map[string]string{
			"Comparison": r.Comparison, "Meaning": r.Meaning,
			"RoundsCompared":        formatInt(r.RoundsCompared),
			"PooledP50DeltaMs":      formatMS(r.PooledP50Delta),
			"PooledP95DeltaMs":      formatMS(r.PooledP95Delta),
			"PooledAvgDeltaMs":      formatMS(r.PooledAvgDelta),
			"RoundP50DeltaMedianMs": formatMS(r.RoundP50DeltaMedian),
			"RoundP50DeltaMinMs":    formatMS(r.RoundP50DeltaMin),
			"RoundP50DeltaMaxMs":    formatMS(r.RoundP50DeltaMax),
			"RoundP95DeltaMedianMs": formatMS(r.RoundP95DeltaMedian),
			"RoundAvgDeltaMedianMs": formatMS(r.RoundAvgDeltaMedian),
		}
	}
	return map[string]string{
		"comparison": r.Comparison, "meaning": r.Meaning,
		"rounds_compared":           formatInt(r.RoundsCompared),
		"pooled_p50_delta_ms":       formatMS(r.PooledP50Delta),
		"pooled_p95_delta_ms":       formatMS(r.PooledP95Delta),
		"pooled_avg_delta_ms":       formatMS(r.PooledAvgDelta),
		"round_p50_delta_median_ms": formatMS(r.RoundP50DeltaMedian),
		"round_p50_delta_min_ms":    formatMS(r.RoundP50DeltaMin),
		"round_p50_delta_max_ms":    formatMS(r.RoundP50DeltaMax),
		"round_p95_delta_median_ms": formatMS(r.RoundP95DeltaMedian),
		"round_avg_delta_median_ms": formatMS(r.RoundAvgDeltaMedian),
	}
}

// ---- config profile ----

// BuildConfigProfile builds a ConfigProfileRow from a resolved config.
func BuildConfigProfile(c *config.Config, endpoint, label string) ConfigProfileRow {
	row := ConfigProfileRow{
		Endpoint:      endpoint,
		Label:         label,
		Provider:      c.Provider,
		API:           c.API,
		Path:          c.Path,
		Model:         c.Model,
		RequestSHA256: c.RequestSHA256(),
		ConfigPath:    c.AbsConfigPath(),
	}
	if c.BaseURL != "" {
		if u, err := url.Parse(c.BaseURL); err == nil && u.Host != "" {
			row.BaseURLHost = u.Host
		} else {
			row.BaseURLHost = c.BaseURL
		}
	}
	if c.Request != nil {
		if v, ok := c.Request["model"].(string); ok && row.Model == "" {
			row.Model = v
		}
		if s, ok := c.Request["stream"].(bool); ok {
			row.Stream = &s
		}
		if mt, ok := c.Request["max_tokens"]; ok {
			row.MaxTokens = mt
		} else if mt, ok := c.Request["max_completion_tokens"]; ok {
			row.MaxTokens = mt
		}
	}
	return row
}

func ConfigProfileMaps(rows []ConfigProfileRow, format Format) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, configProfileMap(r, format))
	}
	return out
}

func configProfileMap(r ConfigProfileRow, format Format) map[string]string {
	if format == FormatPerftest {
		return map[string]string{
			"Endpoint": r.Endpoint, "Label": r.Label, "Provider": r.Provider,
			"BaseUrlHost": r.BaseURLHost, "Path": r.Path, "Model": r.Model,
			"Stream": formatBool(r.Stream, format), "MaxTokens": formatMaxTokens(r.MaxTokens),
			"RequestSha256": r.RequestSHA256, "ConfigPath": r.ConfigPath,
		}
	}
	return map[string]string{
		"endpoint": r.Endpoint, "label": r.Label, "provider": r.Provider,
		"api": r.API, "base_url_host": r.BaseURLHost, "path": r.Path, "model": r.Model,
		"stream": formatBool(r.Stream, format), "max_tokens": formatMaxTokens(r.MaxTokens),
		"request_sha256": r.RequestSHA256, "config_path": r.ConfigPath,
	}
}

// ---- invocations ----

func InvocationMaps(rows []InvocationRow, format Format) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, invocationMap(r, format))
	}
	return out
}

func invocationMap(r InvocationRow, format Format) map[string]string {
	if format == FormatPerftest {
		return map[string]string{
			"Round": formatInt(r.Round), "Endpoint": r.Endpoint, "Label": r.Label,
			"ExitCode": formatInt(r.ExitCode), "StartedAt": r.StartedAt, "EndedAt": r.EndedAt,
			"DurationSec": formatMS(r.DurationSec), "Command": r.Command,
			"JsonlPath": r.JSONLPath, "StdoutPath": r.ProbeLogPath,
		}
	}
	return map[string]string{
		"round": formatInt(r.Round), "endpoint": r.Endpoint, "label": r.Label,
		"exit_code": formatInt(r.ExitCode), "started_at": r.StartedAt, "ended_at": r.EndedAt,
		"duration_sec": formatMS(r.DurationSec), "command": r.Command,
		"jsonl_path": r.JSONLPath, "probe_log_path": r.ProbeLogPath,
	}
}

// HeadersFor returns the field header list for a file kind and format.
func HeadersFor(name string, format Format) []string {
	if format == FormatPerftest {
		switch name {
		case "all_runs":
			return PerftestAllRunsFields
		case "summary", "endpoint_summary":
			return PerftestSummaryFields
		case "round_summary":
			return PerftestSummaryFields
		case "failures":
			return PerftestAllRunsFields
		case "delta_by_round":
			return PerftestDeltaByRoundFields
		case "delta_summary":
			return PerftestDeltaSummaryFields
		case "config_profile":
			return PerftestConfigProfileFields
		case "invocations":
			return PerftestInvocationsFields
		}
	}
	switch name {
	case "all_runs":
		return AllRunsFields
	case "endpoint_summary":
		return SummaryFields
	case "round_summary":
		return SummaryFields
	case "failures":
		return FailuresFields
	case "delta_by_round":
		return DeltaByRoundFields
	case "delta_summary":
		return DeltaSummaryFields
	case "config_profile":
		return ConfigProfileFields
	case "invocations":
		return InvocationsFields
	}
	return nil
}
