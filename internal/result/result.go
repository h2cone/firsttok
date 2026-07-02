// Package result defines the single-probe result model, JSONL encoding, and
// redaction helpers. Result field names use snake_case and match the firsttok
// data contract.
package result

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

// FirstToken describes where and what the first token was.
type FirstToken struct {
	Source  string `json:"source"`
	Preview string `json:"preview"`
}

// Single is one probe attempt, serialized as one JSONL record.
type Single struct {
	Run          int         `json:"run"`
	Warmup       bool        `json:"warmup"`
	Provider     string      `json:"provider"`
	API          string      `json:"api"`
	URL          string      `json:"url"`
	Status       int         `json:"status"`
	OK           bool        `json:"ok"`
	HeadersMS    *float64    `json:"headers_ms"`
	TTFBMS       *float64    `json:"ttfb_ms"`
	FirstEventMS *float64    `json:"first_event_ms"`
	TTFTMS       *float64    `json:"ttft_ms"`
	EventsRead   int         `json:"events_read"`
	BytesRead    int64       `json:"bytes_read"`
	FirstToken   *FirstToken `json:"first_token,omitempty"`
	StartedAt    string      `json:"started_at"`
	Error        string      `json:"error,omitempty"`
}

// Summary is the aggregated tail record appended to each JSONL file.
type Summary struct {
	TimeUnit       string       `json:"time_unit"`
	Runs           int          `json:"runs"`
	SuccessfulRuns int          `json:"successful_runs"`
	FailedRuns     int          `json:"failed_runs"`
	TTFTMS         *SummaryTTFT `json:"ttft_ms,omitempty"`
}

// SummaryTTFT holds ttft percentiles for the summary tail record.
type SummaryTTFT struct {
	Min float64 `json:"min"`
	P50 float64 `json:"p50"`
	Avg float64 `json:"avg"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

// SummaryRecord wraps a Summary for the JSONL tail line: {"summary": {...}}.
type SummaryRecord struct {
	Summary Summary `json:"summary"`
}

// NowISO returns a local-time ISO8601 timestamp with offset, matching
// datetime.now().astimezone().isoformat().
func NowISO() string {
	return time.Now().Format(time.RFC3339)
}

// StartedAtNow returns the current local timestamp string for a probe.
func StartedAtNow() string {
	return time.Now().Format("2006-01-02T15:04:05.000000")
}

// MarshalCompact serializes v as compact JSON (no extra whitespace).
func MarshalCompact(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// RedactURL redacts sensitive query parameters (key, api_key, apikey, token,
// access_token) from a URL string, replacing their values with "***".
func RedactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	if u.RawQuery == "" {
		return s
	}
	values := u.Query()
	sensitive := map[string]bool{
		"key":          true,
		"api_key":      true,
		"apikey":       true,
		"token":        true,
		"access_token": true,
	}
	for k := range values {
		if sensitive[strings.ToLower(k)] {
			values[k] = []string{"***"}
		}
	}
	u.RawQuery = values.Encode()
	return u.String()
}

// PreviewText returns a short single-line preview of text, capped at maxLen.
// Newlines and carriage returns are escaped. The default max is 80.
func PreviewText(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}
	cleaned := strings.ReplaceAll(text, "\r", "\\r")
	cleaned = strings.ReplaceAll(cleaned, "\n", "\\n")
	if len(cleaned) <= maxLen {
		return cleaned
	}
	return cleaned[:maxLen-3] + "..."
}

// MS3 rounds a millisecond duration to 3 decimals, normalizing -0.0 to 0.0.
// Returns nil for a zero/negative sentinel (handled by callers via nil end).
func MS3(v float64) float64 {
	r := round3(v)
	if r == 0 {
		return 0.0
	}
	return r
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// FmtMS formats a nullable millisecond value for tables.
func FmtMS(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.3f", *v)
}
