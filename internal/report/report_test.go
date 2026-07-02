package report

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/firsttok/firsttok/internal/result"
)

func strPtr(f float64) *float64 { return &f }

func makeRun(ok bool, run int, warmup bool, ttft *float64) result.Single {
	r := result.Single{
		Run: run, Warmup: warmup, Provider: "openai", API: "openai-responses",
		URL: "https://x/v1/responses", OK: ok, Status: 200,
		StartedAt: "2026-07-01T00:00:00",
	}
	if ok && ttft != nil {
		r.TTFTMS = ttft
		r.TTFBMS = strPtr(50.0)
		r.HeadersMS = strPtr(20.0)
		r.FirstEventMS = strPtr(60.0)
		r.EventsRead = 3
		r.BytesRead = 100
		r.FirstToken = &result.FirstToken{Source: "choices.*.delta.content", Preview: "Hi"}
	}
	return r
}

func TestJSONLSummaryTailWriteAndSkip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ttft_ep_round1.jsonl")
	runs := []result.Single{
		makeRun(true, 1, true, strPtr(10.0)), // warmup excluded
		makeRun(true, 2, false, strPtr(12.0)),
		makeRun(true, 3, false, strPtr(14.0)),
	}
	summary := ComputeSummary(runs)
	if err := WriteJSONL(path, runs, summary); err != nil {
		t.Fatal(err)
	}
	got, s, err := ReadJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 runs, got %d", len(got))
	}
	if s == nil {
		t.Fatal("summary tail not read")
	}
	if s.Runs != 2 || s.SuccessfulRuns != 2 || s.FailedRuns != 0 {
		t.Errorf("summary wrong: %+v", s)
	}
	if s.TTFTMS == nil {
		t.Fatal("ttft summary missing")
	}
	if s.TTFTMS.Min != 12.0 || s.TTFTMS.Max != 14.0 {
		t.Errorf("summary min/max = %+v", s.TTFTMS)
	}
}

func TestComputeSummaryFailedRun(t *testing.T) {
	runs := []result.Single{
		makeRun(true, 1, false, strPtr(12.0)),
		makeRun(false, 2, false, nil),
	}
	s := ComputeSummary(runs)
	if s.Runs != 2 || s.SuccessfulRuns != 1 || s.FailedRuns != 1 {
		t.Errorf("summary = %+v", s)
	}
}

func TestEndpointKeyDedupRoundSummaries(t *testing.T) {
	runs := []RunRecord{
		{Single: makeRun(true, 1, false, strPtr(10.0)), Endpoint: "a", Label: "a", HasRound: true, Round: 1},
		{Single: makeRun(true, 1, false, strPtr(20.0)), Endpoint: "a", Label: "a", HasRound: true, Round: 2},
		{Single: makeRun(true, 1, false, strPtr(30.0)), Endpoint: "b", Label: "b", HasRound: true, Round: 1},
	}
	es := EndpointSummaries(runs)
	if len(es) != 2 {
		t.Fatalf("expected 2 endpoint summaries, got %d", len(es))
	}
	for _, r := range es {
		if r.Endpoint == "a" && r.RoundsIncluded != 2 {
			t.Errorf("endpoint a rounds_included = %d, want 2", r.RoundsIncluded)
		}
		if r.Endpoint == "b" && r.RoundsIncluded != 1 {
			t.Errorf("endpoint b rounds_included = %d, want 1", r.RoundsIncluded)
		}
	}
	rs := RoundSummaries(runs)
	// 3 (endpoint,round) combos.
	if len(rs) != 3 {
		t.Errorf("expected 3 round summaries, got %d", len(rs))
	}
	for _, r := range rs {
		if r.RoundsIncluded != 1 {
			t.Errorf("round summary rounds_included = %d, want 1", r.RoundsIncluded)
		}
	}
}

func TestBuildComparisonsBusinessAndPairwise(t *testing.T) {
	targets := []Target{
		{Key: "dmxapi", Label: "dmxapi"},
		{Key: "dcs", Label: "dcs"},
		{Key: "dcs-no-plugin-proxy", Label: "dcs-no-plugin-proxy"},
	}
	comps := BuildComparisons(targets)
	// 3 business + 3 pairwise = 6.
	if len(comps) != 6 {
		t.Fatalf("expected 6 comparisons, got %d: %+v", len(comps), comps)
	}
	// First three are business with fixed meanings.
	meanings := []string{
		"DCS plugin+proxy path minus DMXAPI direct",
		"DCS base path minus DMXAPI direct",
		"DCS plugin+proxy path minus DCS no-plugin/proxy path",
	}
	for i, want := range meanings {
		if comps[i].Meaning != want {
			t.Errorf("business meaning %d = %q, want %q", i, comps[i].Meaning, want)
		}
	}
	// Pairwise meaning uses labels.
	found := false
	for _, c := range comps[3:] {
		if c.Meaning == "dcs minus dmxapi" {
			found = true
		}
	}
	if !found {
		t.Errorf("pairwise 'dcs minus dmxapi' missing: %+v", comps[3:])
	}
}

func TestBuildComparisonsPairwiseOnly(t *testing.T) {
	targets := []Target{
		{Key: "alpha", Label: "alpha"},
		{Key: "beta", Label: "beta"},
	}
	comps := BuildComparisons(targets)
	if len(comps) != 1 {
		t.Fatalf("expected 1 pairwise comparison, got %d", len(comps))
	}
	if comps[0].Meaning != "beta minus alpha" {
		t.Errorf("pairwise meaning = %q", comps[0].Meaning)
	}
}

func TestCSVFieldOrderDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "all_runs.csv")
	rows := []map[string]string{
		AllRunsRow(RunRecord{Single: makeRun(true, 1, false, strPtr(10.0)), Endpoint: "ep", Label: "ep", HasRound: true, Round: 1, SourceFile: "f.jsonl"}, FormatDefault),
	}
	if err := WriteCSV(path, AllRunsFields, rows, false); err != nil {
		t.Fatal(err)
	}
	header, records := readCSV(t, path)
	if len(header) != len(AllRunsFields) {
		t.Fatalf("header length mismatch")
	}
	for i, h := range AllRunsFields {
		if header[i] != h {
			t.Errorf("header[%d] = %q, want %q", i, header[i], h)
		}
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func TestCSVQuoteAllPerftest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "all_runs.csv")
	rows := []map[string]string{
		{"Endpoint": "a", "Label": "b"},
	}
	if err := WriteCSV(path, []string{"Endpoint", "Label"}, rows, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	// Every field must be quoted; simplest check: line starts with `"` and contains `,"`.
	if !bytes.HasPrefix(data, []byte(`"`)) {
		t.Errorf("quote-all should wrap fields: %q", data)
	}
	if !bytes.Contains(data, []byte(`","`)) {
		t.Errorf("expected quoted comma separator: %q", data)
	}
	// LF terminator.
	if !bytes.HasSuffix(data, []byte("\n")) || bytes.Contains(data, []byte("\r\n")) {
		t.Errorf("expected LF terminator only: %q", data)
	}
}

func TestHeadersForPerftestEndpointSummary(t *testing.T) {
	// Regression: perftest endpoint_summary must use PascalCase header so rows
	// (which are PascalCase) align.
	h := HeadersFor("endpoint_summary", FormatPerftest)
	if len(h) == 0 || h[0] != "Endpoint" {
		t.Errorf("perftest endpoint_summary header = %v", h)
	}
	hf := HeadersFor("failures", FormatPerftest)
	if len(hf) == 0 || hf[0] != "Endpoint" {
		t.Errorf("perftest failures header = %v", hf)
	}
}

func TestPerftestSummaryRowAligns(t *testing.T) {
	// A perftest summary row must populate Endpoint via the PascalCase header.
	dir := t.TempDir()
	path := filepath.Join(dir, "endpoint_summary.csv")
	row := SummaryRow{Endpoint: "dcs", Label: "dcs", Runs: 1, FailedRuns: 1, RoundsIncluded: 1}
	rows := []map[string]string{summaryMap(row, FormatPerftest)}
	if err := WriteCSV(path, HeadersFor("endpoint_summary", FormatPerftest), rows, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte(`"dcs"`)) {
		t.Errorf("endpoint value missing in perftest summary: %q", data)
	}
}

func TestRedactionInResult(t *testing.T) {
	got := result.RedactURL("https://x/v1/messages?key=secret&model=m")
	if contains(got, "secret") {
		t.Errorf("key value not redacted: %q", got)
	}
	if !contains(got, "model=m") {
		t.Errorf("model redacted: %q", got)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

func readCSV(t *testing.T, path string) ([]string, [][]string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return header, records
}
