// Package runset orchestrates multi-round bench and compare runs: it executes
// probes in-process, writes per-round JSONL files, and emits the full report
// directory (CSVs, metadata.jsonl, report.txt).
package runset

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/firsttok/firsttok/internal/config"
	"github.com/firsttok/firsttok/internal/probe"
	"github.com/firsttok/firsttok/internal/report"
	"github.com/firsttok/firsttok/internal/result"
)

// Settings controls a bench/compare run.
type Settings struct {
	Rounds           int
	Warmup           int
	Repeat           int
	TimeoutSec       int
	PauseSeconds     float64
	FixedOrder       bool
	Seed             int64
	NoValidateStream bool
	Format           report.Format
	OutputDir        string
	OutputDirSet     bool
	FailOnRunFailure bool
	StopOnFailure    bool
}

// Defaults applied when zero.
const (
	DefaultRounds     = 5
	DefaultWarmup     = 2
	DefaultRepeat     = 20
	DefaultTimeoutSec = 120
)

// Target is a resolved compare/bench target.
type Target struct {
	report.Target
	Config *config.Config
}

// jsonlName builds ttft_<endpoint>_round<N>.jsonl.
func jsonlName(endpoint string, round int) string {
	return fmt.Sprintf("ttft_%s_round%d.jsonl", endpoint, round)
}

var jsonlRe = regexp.MustCompile(`^ttft_(.+)_round(\d+)\.jsonl$`)

// ParseJSONLName extracts (endpoint, round) from a ttft_<endpoint>_round<N>.jsonl name.
func ParseJSONLName(name string) (endpoint string, round int, ok bool) {
	m := jsonlRe.FindStringSubmatch(name)
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0, false
	}
	return m[1], n, true
}

// Stamp returns the local-time YYYYMMDD_HHMMSS stamp.
func Stamp(t time.Time) string {
	return t.Format("20060102_150405")
}

// ResolveOutputDir returns the output directory, creating a timestamped default
// under ttft_runs/ when not explicitly set.
func ResolveOutputDir(s *Settings) string {
	if s.OutputDirSet && s.OutputDir != "" {
		return s.OutputDir
	}
	return filepath.Join("ttft_runs", Stamp(time.Now()))
}

// prepareProbe builds the probe options from a resolved config, validating stream.
func prepareProbe(c *config.Config, timeoutSec int, noValidate bool) (probe.Options, error) {
	if err := config.ValidateStream(c.API, c.Request, noValidate); err != nil {
		return probe.Options{}, err
	}
	url, err := config.BuildURL(c)
	if err != nil {
		return probe.Options{}, err
	}
	return probe.Options{
		URL:         url,
		Headers:     c.Headers,
		Body:        c.BodyBytes(),
		API:         c.API,
		CustomPaths: c.FirstTokenPaths,
		VerifySSL:   c.VerifySSL,
		Timeout:     time.Duration(timeoutSec) * time.Second,
	}, nil
}

// RunTargetOnce executes warmup+repeat probes for one target in one round,
// writes the JSONL file, and returns the invocation row + run records.
func RunTargetOnce(t Target, s Settings, round int, dir string) (report.InvocationRow, []report.RunRecord, error) {
	started := time.Now()
	startedAt := started.Format(time.RFC3339Nano)
	opts, err := prepareProbe(t.Config, s.TimeoutSec, s.NoValidateStream)
	if err != nil {
		return report.InvocationRow{}, nil, err
	}

	total := s.Warmup + s.Repeat
	runs := make([]result.Single, 0, total)
	for i := 1; i <= total; i++ {
		warmup := i <= s.Warmup
		r := probe.Run(opts, i, warmup, t.Config.Provider, t.Config.API)
		runs = append(runs, r)
	}

	jsonlPath := filepath.Join(dir, jsonlName(t.Key, round))
	summary := report.ComputeSummary(runs)
	if err := report.WriteJSONL(jsonlPath, runs, summary); err != nil {
		return report.InvocationRow{}, nil, err
	}

	ended := time.Now()
	exitCode := 0
	if summary.FailedRuns > 0 || summary.SuccessfulRuns != summary.Runs {
		exitCode = 2
	}
	dur := ended.Sub(started).Seconds()
	durPtr := &dur

	inv := report.InvocationRow{
		Round:       round,
		Endpoint:    t.Key,
		Label:       t.Label,
		ExitCode:    exitCode,
		StartedAt:   startedAt,
		EndedAt:     ended.Format(time.RFC3339Nano),
		DurationSec: durPtr,
		Command:     synthCommand(t, s, jsonlPath),
		JSONLPath:   jsonlPath,
	}

	// Build run records enriched with endpoint/label/round/source_file.
	records := make([]report.RunRecord, 0, len(runs))
	for _, r := range runs {
		rec := report.RunRecord{Single: r, Endpoint: t.Key, Label: t.Label, HasRound: true, Round: round, SourceFile: jsonlPath}
		records = append(records, rec)
	}
	return inv, records, nil
}

func synthCommand(t Target, s Settings, jsonlPath string) string {
	parts := []string{"firsttok", "run", "--config", t.Config.ConfigPath,
		"--warmup", strconv.Itoa(s.Warmup),
		"--repeat", strconv.Itoa(s.Repeat),
		"--timeout-sec", strconv.Itoa(s.TimeoutSec),
		"--output-jsonl", jsonlPath,
	}
	if s.NoValidateStream {
		parts = append(parts, "--no-validate-stream")
	}
	if !t.Config.VerifySSL {
		parts = append(parts, "--insecure")
	}
	return strings.Join(parts, " ")
}

// RunBench executes a single-target bench run.
func RunBench(t Target, s Settings) (string, error) {
	dir := ResolveOutputDir(&s)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	targets := []Target{t}
	return dir, runPlan(targets, s, dir, false)
}

// RunCompare executes a multi-target compare run.
func RunCompare(targets []Target, s Settings) (string, error) {
	dir := ResolveOutputDir(&s)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, runPlan(targets, s, dir, true)
}

// runPlan executes the rounds×targets matrix and writes all report artifacts.
func runPlan(targets []Target, s Settings, dir string, isCompare bool) error {
	rng := newRNG(s.Seed)
	var invocations []report.InvocationRow
	var allRuns []report.RunRecord

	rounds := s.Rounds
	for round := 1; round <= rounds; round++ {
		order := make([]int, len(targets))
		for i := range targets {
			order[i] = i
		}
		if !s.FixedOrder && len(targets) > 1 {
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		}
		for _, idx := range order {
			t := targets[idx]
			inv, records, err := RunTargetOnce(t, s, round, dir)
			if err != nil {
				if isConfigErr(err) {
					return err
				}
				// Record as a failed invocation with exit code 1.
				inv = report.InvocationRow{
					Round: round, Endpoint: t.Key, Label: t.Label, ExitCode: 1,
					StartedAt: time.Now().Format(time.RFC3339Nano),
					EndedAt:   time.Now().Format(time.RFC3339Nano),
					Command:   synthCommand(t, s, filepath.Join(dir, jsonlName(t.Key, round))),
				}
				invocations = append(invocations, inv)
				if s.StopOnFailure {
					return fmt.Errorf("target %s round %d failed: %v", t.Key, round, err)
				}
				continue
			}
			invocations = append(invocations, inv)
			allRuns = append(allRuns, records...)
			if s.StopOnFailure && inv.ExitCode != 0 {
				return fmt.Errorf("target %s round %d exited with code %d", t.Key, round, inv.ExitCode)
			}
			if s.PauseSeconds > 0 {
				time.Sleep(time.Duration(s.PauseSeconds * float64(time.Second)))
			}
		}
	}

	// Build report artifacts.
	return writeArtifacts(targets, s, dir, isCompare, invocations, allRuns)
}

func newRNG(seed int64) *rand.Rand {
	if seed < 0 {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return rand.New(rand.NewSource(seed))
}

func isConfigErr(err error) bool {
	return config.IsConfigError(err)
}

// writeArtifacts generates all CSVs, metadata.jsonl and report.txt.
func writeArtifacts(targets []Target, s Settings, dir string, isCompare bool, invocations []report.InvocationRow, allRuns []report.RunRecord) error {
	format := s.Format
	quoteAll := format == report.FormatPerftest

	// all_runs.csv
	if err := report.WriteCSV(filepath.Join(dir, "all_runs.csv"), report.HeadersFor("all_runs", format), report.AllRunsMaps(allRuns, format), quoteAll); err != nil {
		return err
	}

	// endpoint_summary.csv + round_summary.csv
	endpointSummaries := report.EndpointSummaries(allRuns)
	if err := report.WriteCSV(filepath.Join(dir, "endpoint_summary.csv"), report.HeadersFor("endpoint_summary", format), report.SummaryMaps(endpointSummaries, format), quoteAll); err != nil {
		return err
	}
	roundSummaries := report.RoundSummaries(allRuns)
	if err := report.WriteCSV(filepath.Join(dir, "round_summary.csv"), report.HeadersFor("round_summary", format), report.SummaryMaps(roundSummaries, format), quoteAll); err != nil {
		return err
	}

	// failures.csv
	if err := report.WriteCSV(filepath.Join(dir, "failures.csv"), report.HeadersFor("failures", format), report.FailuresMaps(allRuns, format), quoteAll); err != nil {
		return err
	}

	// config_profile.csv
	var profileRows []report.ConfigProfileRow
	for _, t := range targets {
		profileRows = append(profileRows, report.BuildConfigProfile(t.Config, t.Key, t.Label))
	}
	if err := report.WriteCSV(filepath.Join(dir, "config_profile.csv"), report.HeadersFor("config_profile", format), report.ConfigProfileMaps(profileRows, format), quoteAll); err != nil {
		return err
	}

	// invocations.csv
	if err := report.WriteCSV(filepath.Join(dir, "invocations.csv"), report.HeadersFor("invocations", format), report.InvocationMaps(invocations, format), quoteAll); err != nil {
		return err
	}

	// compare-only delta CSVs
	var deltaSummaryRows []report.DeltaSummaryRow
	if isCompare {
		rt := make([]report.Target, len(targets))
		for i, t := range targets {
			rt[i] = t.Target
		}
		comps := report.BuildComparisons(rt)
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

	// metadata.jsonl
	if err := writeMetadata(targets, s, dir, isCompare); err != nil {
		return err
	}

	// report.txt
	if err := writeReportTxt(targets, s, dir, isCompare, allRuns, endpointSummaries, roundSummaries, deltaSummaryRows); err != nil {
		return err
	}

	// request_sha256 mismatch warning (compare).
	if isCompare {
		warnIfRequestMismatch(profileRows)
	}

	// Final exit-code semantics for bench/compare:
	//   default: 0 even if invocations have exit_code=2.
	//   --fail-on-run-failure: 2 if any non-warmup probe failed.
	if s.FailOnRunFailure {
		for _, inv := range invocations {
			if inv.ExitCode == 2 {
				return &failOnRunError{}
			}
		}
	}
	return nil
}

type failOnRunError struct{}

func (e *failOnRunError) Error() string { return "non-warmup probe failure" }
func (e *failOnRunError) ExitCode() int { return 2 }

// FailOnRunError reports a probe failure for --fail-on-run-failure.
func IsFailOnRunError(err error) bool {
	var f *failOnRunError
	return errors.As(err, &f)
}

func writeMetadata(targets []Target, s Settings, dir string, isCompare bool) error {
	meta := report.Metadata{
		GeneratedAt:   result.NowISO(),
		OutputDir:     dir,
		AggregateOnly: false,
		Rounds:        s.Rounds,
		Repeat:        s.Repeat,
		Warmup:        s.Warmup,
		TimeoutSec:    s.TimeoutSec,
		PauseSeconds:  s.PauseSeconds,
		FixedOrder:    s.FixedOrder,
		Seed:          int(s.Seed),
		TTFTScript:    "firsttok",
	}
	if isCompare {
		meta.Endpoints = make([]report.MetadataEndpoint, len(targets))
		for i, t := range targets {
			meta.Endpoints[i] = report.MetadataEndpoint{Key: t.Key, Label: t.Label, Config: t.Config.AbsConfigPath(), Chain: t.Chain}
		}
	} else {
		t := targets[0]
		meta.Target = &report.MetadataTarget{Key: t.Key, Label: t.Label, Config: t.Config.AbsConfigPath()}
	}
	data, err := result.MarshalCompact(meta)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "metadata.jsonl"), data, 0o644)
}

func writeReportTxt(targets []Target, s Settings, dir string, isCompare bool, allRuns []report.RunRecord, endpointSummaries, roundSummaries []report.SummaryRow, deltaSummaryRows []report.DeltaSummaryRow) error {
	var profileRows []report.ConfigProfileRow
	for _, t := range targets {
		profileRows = append(profileRows, report.BuildConfigProfile(t.Config, t.Key, t.Label))
	}
	order := "randomized"
	if s.FixedOrder || len(targets) <= 1 {
		order = "fixed"
	}
	method := fmt.Sprintf("rounds=%d, repeat=%d, warmup=%d, timeout_sec=%d, order=%s",
		s.Rounds, s.Repeat, s.Warmup, s.TimeoutSec, order)

	rd := &report.ReportData{
		IsCompare:       isCompare,
		GeneratedAt:     result.NowISO(),
		OutputDir:       dir,
		Method:          method,
		ConfigProfiles:  profileRows,
		EndpointSummary: endpointSummaries,
		RoundSummary:    roundSummaries,
		DeltaSummary:    deltaSummaryRows,
		GeneratedFiles:  generatedFiles(isCompare),
	}
	if isCompare {
		for _, t := range targets {
			rd.Targets = append(rd.Targets, t.Target)
		}
	}
	rd.Failures = collectFailures(allRuns)
	return os.WriteFile(filepath.Join(dir, "report.txt"), []byte(rd.Render()), 0o644)
}

func collectFailures(allRuns []report.RunRecord) []report.RunRecord {
	var out []report.RunRecord
	for _, r := range allRuns {
		if !r.Warmup && !r.OK {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Endpoint != out[j].Endpoint {
			return out[i].Endpoint < out[j].Endpoint
		}
		return out[i].Round < out[j].Round
	})
	return out
}

func generatedFiles(isCompare bool) []string {
	files := []string{"all_runs.csv", "endpoint_summary.csv", "round_summary.csv"}
	if isCompare {
		files = append(files, "delta_by_round.csv", "delta_summary.csv")
	}
	files = append(files, "failures.csv", "config_profile.csv", "invocations.csv", "metadata.jsonl", "report.txt")
	return files
}

func warnIfRequestMismatch(rows []report.ConfigProfileRow) {
	if len(rows) < 2 {
		return
	}
	first := rows[0].RequestSHA256
	for _, r := range rows[1:] {
		if r.RequestSHA256 != first {
			fmt.Fprintf(os.Stderr, "warning: request_sha256 differs across targets; delta may mix prompt/model differences\n")
			return
		}
	}
}
