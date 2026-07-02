package runset

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/firsttok/firsttok/internal/report"
)

// RunReport regenerates all CSVs and report.txt in an existing result
// directory from its raw JSONL files, metadata.jsonl and config profile. No
// network requests are made.
func RunReport(dir string, format report.Format) error {
	meta, err := readMetadata(filepath.Join(dir, "metadata.jsonl"))
	if err != nil {
		return fmt.Errorf("read metadata.jsonl: %v", err)
	}
	isCompare := meta.IsCompare()

	// Collect run records from all ttft_*_round*.jsonl files.
	records, err := readAllJSONL(dir, meta)
	if err != nil {
		return err
	}

	// Read existing config profile (the "available config profile").
	profileRows, err := readConfigProfile(dir)
	if err != nil {
		return err
	}

	// Build targets list for report.txt and delta comparisons.
	var targets []report.Target
	if isCompare {
		for _, e := range meta.Endpoints {
			targets = append(targets, report.Target{Key: e.Key, Label: e.Label, Config: e.Config, Chain: e.Chain})
		}
	} else if meta.Target != nil {
		targets = append(targets, report.Target{Key: meta.Target.Key, Label: meta.Target.Label, Config: meta.Target.Config})
	}

	return writeOfflineArtifacts(dir, format, isCompare, meta, records, profileRows, targets)
}

func writeOfflineArtifacts(dir string, format report.Format, isCompare bool, meta *report.Metadata, allRuns []report.RunRecord, profileRows []report.ConfigProfileRow, targets []report.Target) error {
	quoteAll := format == report.FormatPerftest

	if err := report.WriteCSV(filepath.Join(dir, "all_runs.csv"), report.HeadersFor("all_runs", format), report.AllRunsMaps(allRuns, format), quoteAll); err != nil {
		return err
	}
	endpointSummaries := report.EndpointSummaries(allRuns)
	if err := report.WriteCSV(filepath.Join(dir, "endpoint_summary.csv"), report.HeadersFor("endpoint_summary", format), report.SummaryMaps(endpointSummaries, format), quoteAll); err != nil {
		return err
	}
	roundSummaries := report.RoundSummaries(allRuns)
	if err := report.WriteCSV(filepath.Join(dir, "round_summary.csv"), report.HeadersFor("round_summary", format), report.SummaryMaps(roundSummaries, format), quoteAll); err != nil {
		return err
	}
	if err := report.WriteCSV(filepath.Join(dir, "failures.csv"), report.HeadersFor("failures", format), report.FailuresMaps(allRuns, format), quoteAll); err != nil {
		return err
	}
	if err := report.WriteCSV(filepath.Join(dir, "config_profile.csv"), report.HeadersFor("config_profile", format), report.ConfigProfileMaps(profileRows, format), quoteAll); err != nil {
		return err
	}

	// invocations.csv is not reconstructable from JSONL alone (no per-target
	// exit codes/durations). Preserve the existing file if present; otherwise
	// emit an empty file with the header.
	invPath := filepath.Join(dir, "invocations.csv")
	if !fileExists(invPath) {
		if err := report.WriteCSV(invPath, report.HeadersFor("invocations", format), nil, quoteAll); err != nil {
			return err
		}
	}

	var deltaSummaryRows []report.DeltaSummaryRow
	if isCompare {
		comps := report.BuildComparisons(targets)
		rounds := report.DistinctRounds(allRuns)
		deltaByRound := report.DeltaByRound(roundSummaries, comps, rounds)
		report.SortDeltaRows(deltaByRound)
		if err := report.WriteCSV(filepath.Join(dir, "delta_by_round.csv"), report.HeadersFor("delta_by_round", format), report.DeltaByRoundMaps(deltaByRound, format), quoteAll); err != nil {
			return err
		}
		deltaSummaryRows = report.DeltaSummary(endpointSummaries, deltaByRound, comps)
		if err := report.WriteCSV(filepath.Join(dir, "delta_summary.csv"), report.HeadersFor("delta_summary", format), report.DeltaSummaryMaps(deltaSummaryRows, format), quoteAll); err != nil {
			return err
		}
	}

	// report.txt
	order := "randomized"
	if meta.FixedOrder || len(targets) <= 1 {
		order = "fixed"
	}
	method := fmt.Sprintf("rounds=%d, repeat=%d, warmup=%d, timeout_sec=%d, order=%s",
		meta.Rounds, meta.Repeat, meta.Warmup, meta.TimeoutSec, order)
	rd := &report.ReportData{
		IsCompare:       isCompare,
		GeneratedAt:     meta.GeneratedAt,
		OutputDir:       dir,
		Method:          method,
		Targets:         targets,
		ConfigProfiles:  profileRows,
		EndpointSummary: endpointSummaries,
		RoundSummary:    roundSummaries,
		DeltaSummary:    deltaSummaryRows,
		GeneratedFiles:  generatedFiles(isCompare),
		Failures:        collectFailures(allRuns),
	}
	return os.WriteFile(filepath.Join(dir, "report.txt"), []byte(rd.Render()), 0o644)
}

func readMetadata(path string) (*report.Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta report.Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func readAllJSONL(dir string, meta *report.Metadata) ([]report.RunRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	// label lookup by endpoint key.
	labels := map[string]string{}
	if meta.Target != nil {
		labels[meta.Target.Key] = meta.Target.Label
	}
	for _, e := range meta.Endpoints {
		labels[e.Key] = e.Label
	}

	var records []report.RunRecord
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		endpoint, round, ok := ParseJSONLName(e.Name())
		if !ok {
			continue
		}
		runs, _, err := report.ReadJSONL(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			rec := report.RunRecord{
				Single:     r,
				Endpoint:   endpoint,
				Label:      labels[endpoint],
				HasRound:   true,
				Round:      round,
				SourceFile: e.Name(),
			}
			records = append(records, rec)
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Endpoint != records[j].Endpoint {
			return records[i].Endpoint < records[j].Endpoint
		}
		if records[i].Round != records[j].Round {
			return records[i].Round < records[j].Round
		}
		return records[i].Run < records[j].Run
	})
	return records, nil
}

// readConfigProfile reads config_profile.csv back into rows, detecting default
// vs perftest field naming from the header. Missing file → minimal rows from
// metadata.
func readConfigProfile(dir string) ([]report.ConfigProfileRow, error) {
	path := filepath.Join(dir, "config_profile.csv")
	f, err := os.Open(path)
	if err != nil {
		// Fallback: build minimal rows from metadata.
		return nil, nil
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(h)] = i
	}
	var rows []report.ConfigProfileRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		row := ConfigProfileRowFromCSV(rec, idx)
		rows = append(rows, row)
	}
	return rows, nil
}

// ConfigProfileRowFromCSV builds a ConfigProfileRow from a CSV record using a
// lowercase header index.
func ConfigProfileRowFromCSV(rec []string, idx map[string]int) report.ConfigProfileRow {
	get := func(name string) string {
		if i, ok := idx[name]; ok && i < len(rec) {
			return rec[i]
		}
		return ""
	}
	row := report.ConfigProfileRow{
		Endpoint:      get("endpoint"),
		Label:         get("label"),
		Provider:      get("provider"),
		API:           get("api"),
		BaseURLHost:   get("base_url_host"),
		Path:          get("path"),
		Model:         get("model"),
		MaxTokens:     nil,
		RequestSHA256: get("request_sha256"),
		ConfigPath:    get("config_path"),
	}
	if s := get("stream"); s == "true" || s == "True" {
		b := true
		row.Stream = &b
	} else if s == "false" || s == "False" {
		b := false
		row.Stream = &b
	}
	if mt := get("max_tokens"); mt != "" {
		row.MaxTokens = mt
	}
	return row
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
