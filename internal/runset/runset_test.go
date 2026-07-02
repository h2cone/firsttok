package runset

import (
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/firsttok/firsttok/internal/config"
	"github.com/firsttok/firsttok/internal/report"
)

func startStreamServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(10 * time.Millisecond)
		// Anthropic-style SSE.
		io.WriteString(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"Hi\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(5 * time.Millisecond)
		io.WriteString(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

func writeConfig(t *testing.T, dir, name, baseURL string) string {
	content := `{"provider":"claude","base_url":"` + baseURL + `","api_key":"test","path":"/v1/messages","request":{"model":"claude","max_tokens":10,"messages":[{"role":"user","content":"hi"}],"stream":true}}`
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadTargets(t *testing.T, paths []string) []Target {
	used := map[string]bool{}
	var targets []Target
	for _, p := range paths {
		cfg, err := config.Load(p, &config.CLIOverrides{})
		if err != nil {
			t.Fatal(err)
		}
		base := config.EndpointKeyFromName(filepath.Base(p))
		key := config.UniqueKey(base, used)
		targets = append(targets, Target{
			Target: report.Target{Key: key, Label: key, Config: p},
			Config: cfg,
		})
	}
	return targets
}

func TestRunCompareEndToEnd(t *testing.T) {
	srv := startStreamServer(t)
	defer srv.Close()

	dir := t.TempDir()
	paths := []string{
		writeConfig(t, dir, "ttft.claude.dmxapi.json", srv.URL),
		writeConfig(t, dir, "ttft.claude.dcs.json", srv.URL),
		writeConfig(t, dir, "ttft.claude.dcs-no-plugin-proxy.json", srv.URL),
	}
	targets := loadTargets(t, paths)

	settings := Settings{
		Rounds: 2, Warmup: 1, Repeat: 3, TimeoutSec: 5,
		Seed: 1, FixedOrder: true, Format: report.FormatDefault,
		OutputDir: filepath.Join(t.TempDir(), "compare"), OutputDirSet: true,
	}
	outDir, err := RunCompare(targets, settings)
	if err != nil {
		t.Fatalf("RunCompare failed: %v", err)
	}

	// Expected files.
	for _, f := range []string{"all_runs.csv", "endpoint_summary.csv", "round_summary.csv",
		"delta_by_round.csv", "delta_summary.csv", "failures.csv", "config_profile.csv",
		"invocations.csv", "metadata.jsonl", "report.txt"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	// Per-round JSONL files: 3 endpoints * 2 rounds = 6.
	entries, _ := os.ReadDir(outDir)
	jsonlCount := 0
	for _, e := range entries {
		if _, _, ok := ParseJSONLName(e.Name()); ok {
			jsonlCount++
		}
	}
	if jsonlCount != 6 {
		t.Errorf("expected 6 jsonl files, got %d", jsonlCount)
	}

	// metadata indicates compare.
	meta, err := readMetadata(filepath.Join(outDir, "metadata.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !meta.IsCompare() {
		t.Error("metadata should indicate compare")
	}
	if len(meta.Endpoints) != 3 {
		t.Errorf("endpoints = %d", len(meta.Endpoints))
	}

	// report.txt contains comparison header.
	rep, _ := os.ReadFile(filepath.Join(outDir, "report.txt"))
	if !contains(string(rep), "TTFT comparison report") {
		t.Error("report.txt missing comparison header")
	}
	if !contains(string(rep), "Delta summary") {
		t.Error("report.txt missing delta summary section")
	}

	// all_runs.csv: warmup excluded from stats but present in all_runs.
	allRuns := readCSVFile(t, filepath.Join(outDir, "all_runs.csv"))
	// 3 endpoints * 2 rounds * (1 warmup + 3 repeat) = 24 rows.
	if len(allRuns) != 24 {
		t.Errorf("all_runs rows = %d, want 24", len(allRuns))
	}
}

func TestRunBenchEndToEnd(t *testing.T) {
	srv := startStreamServer(t)
	defer srv.Close()

	dir := t.TempDir()
	p := writeConfig(t, dir, "ttft.claude.dmxapi.json", srv.URL)
	cfg, err := config.Load(p, &config.CLIOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	key := config.EndpointKeyFromName(filepath.Base(p))
	target := Target{
		Target: report.Target{Key: key, Label: key, Config: p},
		Config: cfg,
	}
	settings := Settings{
		Rounds: 2, Warmup: 0, Repeat: 2, TimeoutSec: 5,
		FixedOrder: true, Format: report.FormatDefault,
		OutputDir: filepath.Join(t.TempDir(), "bench"), OutputDirSet: true,
	}
	outDir, err := RunBench(target, settings)
	if err != nil {
		t.Fatalf("RunBench failed: %v", err)
	}
	// bench should NOT have delta files.
	if _, err := os.Stat(filepath.Join(outDir, "delta_summary.csv")); err == nil {
		t.Error("bench should not produce delta_summary.csv")
	}
	rep, _ := os.ReadFile(filepath.Join(outDir, "report.txt"))
	if !contains(string(rep), "TTFT single config report") {
		t.Error("bench report.txt missing single config header")
	}
	meta, _ := readMetadata(filepath.Join(outDir, "metadata.jsonl"))
	if meta.IsCompare() {
		t.Error("bench metadata should not be compare")
	}
}

func TestRunReportRegenerates(t *testing.T) {
	srv := startStreamServer(t)
	defer srv.Close()

	dir := t.TempDir()
	paths := []string{
		writeConfig(t, dir, "ttft.claude.dmxapi.json", srv.URL),
		writeConfig(t, dir, "ttft.claude.dcs.json", srv.URL),
	}
	targets := loadTargets(t, paths)
	settings := Settings{
		Rounds: 2, Warmup: 0, Repeat: 2, TimeoutSec: 5,
		Seed: 1, FixedOrder: true, Format: report.FormatDefault,
		OutputDir: filepath.Join(t.TempDir(), "compare2"), OutputDirSet: true,
	}
	outDir, err := RunCompare(targets, settings)
	if err != nil {
		t.Fatal(err)
	}

	// Snapshot all_runs.csv content, then regenerate and compare.
	orig := readCSVFile(t, filepath.Join(outDir, "all_runs.csv"))
	if err := RunReport(outDir, report.FormatDefault); err != nil {
		t.Fatalf("RunReport failed: %v", err)
	}
	regen := readCSVFile(t, filepath.Join(outDir, "all_runs.csv"))
	if len(orig) != len(regen) {
		t.Errorf("regenerated all_runs rows = %d, want %d", len(regen), len(orig))
	}
	// report.txt still present.
	if _, err := os.Stat(filepath.Join(outDir, "report.txt")); err != nil {
		t.Error("report.txt missing after regen")
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && filepathExists(s, sub) }

func filepathExists(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func readCSVFile(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// drop header
	if len(rows) > 0 {
		return rows[1:]
	}
	return rows
}
