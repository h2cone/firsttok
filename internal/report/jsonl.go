package report

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/firsttok/firsttok/internal/result"
	"github.com/firsttok/firsttok/internal/stats"
)

// WriteJSONL writes run records as compact JSON lines followed by a single
// {"summary": {...}} tail line.
func WriteJSONL(path string, runs []result.Single, summary result.Summary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, r := range runs {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		w.Write(data)
		w.WriteByte('\n')
	}
	tail, err := json.Marshal(result.SummaryRecord{Summary: summary})
	if err != nil {
		return err
	}
	w.Write(tail)
	w.WriteByte('\n')
	return w.Flush()
}

// ComputeSummary builds the summary tail record from non-warmup runs.
func ComputeSummary(runs []result.Single) result.Summary {
	var nonWarmup []result.Single
	for _, r := range runs {
		if !r.Warmup {
			nonWarmup = append(nonWarmup, r)
		}
	}
	s := result.Summary{
		TimeUnit:       "ms",
		Runs:           len(nonWarmup),
		SuccessfulRuns: 0,
		FailedRuns:     0,
	}
	var ttft []float64
	for _, r := range nonWarmup {
		if !r.OK {
			s.FailedRuns++
			continue
		}
		if r.TTFTMS == nil {
			s.FailedRuns++
			continue
		}
		s.SuccessfulRuns++
		ttft = append(ttft, *r.TTFTMS)
	}
	if len(ttft) > 0 {
		min := stats.Min(ttft)
		p50 := stats.Percentile(ttft, 0.50)
		avg := stats.Mean(ttft)
		p95 := stats.Percentile(ttft, 0.95)
		max := stats.Max(ttft)
		s.TTFTMS = &result.SummaryTTFT{
			Min: *stats.RoundNullable(min),
			P50: *stats.RoundNullable(p50),
			Avg: *stats.RoundNullable(&avg),
			P95: *stats.RoundNullable(p95),
			Max: *stats.RoundNullable(max),
		}
	}
	return s
}

// ReadJSONL reads run records from a JSONL file, skipping the summary tail line.
// Returns the parsed runs and the summary record if present.
func ReadJSONL(path string) ([]result.Single, *result.Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var runs []result.Single
	var summary *result.Summary
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		// Detect the summary tail record.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(line, &probe); err != nil {
			continue
		}
		if _, ok := probe["summary"]; ok {
			var rec result.SummaryRecord
			if err := json.Unmarshal(line, &rec); err == nil {
				summary = &rec.Summary
			}
			continue
		}
		var r result.Single
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		runs = append(runs, r)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	return runs, summary, nil
}
