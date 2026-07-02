package report

import (
	"github.com/firsttok/firsttok/internal/stats"
)

// SummaryRow holds the aggregated fields written to endpoint_summary.csv and
// round_summary.csv.
type SummaryRow struct {
	Endpoint       string
	Label          string
	Round          *int
	RoundsIncluded int
	Runs           int
	SuccessfulRuns int
	FailedRuns     int
	FailureRatePct *float64
	TTFTMin        *float64
	TTFTP25        *float64
	TTFTP50        *float64
	TTFTP75        *float64
	TTFTP90        *float64
	TTFTP95        *float64
	TTFTP99        *float64
	TTFTMax        *float64
	TTFTAvg        *float64
	TTFTStdDev     *float64
	TTFBP50        *float64
	HeadersP50     *float64
	FirstEventP50  *float64
	EventsReadP50  *float64
	BytesReadP50   *float64
}

// BuildSummaryRow aggregates a slice of (already-scoped, non-warmup) run
// records into a summary row. round is nil for endpoint_summary, non-nil for
// round_summary.
func BuildSummaryRow(endpoint, label string, round *int, runs []RunRecord) SummaryRow {
	row := SummaryRow{Endpoint: endpoint, Label: label, Round: round}
	row.Runs = len(runs)
	rounds := map[int]bool{}
	var ttft, ttfb, headers, firstEvent []float64
	var eventsRead, bytesRead []float64
	for _, r := range runs {
		if r.HasRound {
			rounds[r.Round] = true
		}
		if !r.OK || r.TTFTMS == nil {
			row.FailedRuns++
			continue
		}
		row.SuccessfulRuns++
		ttft = append(ttft, *r.TTFTMS)
		if r.TTFBMS != nil {
			ttfb = append(ttfb, *r.TTFBMS)
		}
		if r.HeadersMS != nil {
			headers = append(headers, *r.HeadersMS)
		}
		if r.FirstEventMS != nil {
			firstEvent = append(firstEvent, *r.FirstEventMS)
		}
		eventsRead = append(eventsRead, float64(r.EventsRead))
		bytesRead = append(bytesRead, float64(r.BytesRead))
	}
	if round == nil {
		row.RoundsIncluded = len(rounds)
	} else {
		row.RoundsIncluded = 1
	}
	if row.Runs > 0 {
		fr := 100.0 * float64(row.FailedRuns) / float64(row.Runs)
		row.FailureRatePct = stats.RoundNullable(&fr)
	}
	row.TTFTMin = stats.RoundNullable(stats.Min(ttft))
	row.TTFTP25 = stats.RoundNullable(stats.Percentile(ttft, 0.25))
	row.TTFTP50 = stats.RoundNullable(stats.Percentile(ttft, 0.50))
	row.TTFTP75 = stats.RoundNullable(stats.Percentile(ttft, 0.75))
	row.TTFTP90 = stats.RoundNullable(stats.Percentile(ttft, 0.90))
	row.TTFTP95 = stats.RoundNullable(stats.Percentile(ttft, 0.95))
	row.TTFTP99 = stats.RoundNullable(stats.Percentile(ttft, 0.99))
	row.TTFTMax = stats.RoundNullable(stats.Max(ttft))
	if len(ttft) > 0 {
		avg := stats.Mean(ttft)
		row.TTFTAvg = stats.RoundNullable(&avg)
	}
	row.TTFTStdDev = stats.RoundNullable(stats.StdDev(ttft))
	row.TTFBP50 = stats.RoundNullable(stats.Percentile(ttfb, 0.50))
	row.HeadersP50 = stats.RoundNullable(stats.Percentile(headers, 0.50))
	row.FirstEventP50 = stats.RoundNullable(stats.Percentile(firstEvent, 0.50))
	row.EventsReadP50 = stats.RoundNullable(stats.Percentile(eventsRead, 0.50))
	row.BytesReadP50 = stats.RoundNullable(stats.Percentile(bytesRead, 0.50))
	return row
}

// FilterNonWarmup returns the non-warmup run records.
func FilterNonWarmup(runs []RunRecord) []RunRecord {
	out := make([]RunRecord, 0, len(runs))
	for _, r := range runs {
		if !r.Warmup {
			out = append(out, r)
		}
	}
	return out
}

// GroupByEndpoint groups non-warmup runs by endpoint key.
func GroupByEndpoint(runs []RunRecord) map[string][]RunRecord {
	out := map[string][]RunRecord{}
	for _, r := range runs {
		out[r.Endpoint] = append(out[r.Endpoint], r)
	}
	return out
}

// DistinctRounds returns the sorted distinct round numbers present in runs.
func DistinctRounds(runs []RunRecord) []int {
	seen := map[int]bool{}
	var out []int
	for _, r := range runs {
		if r.HasRound && !seen[r.Round] {
			seen[r.Round] = true
			out = append(out, r.Round)
		}
	}
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j-1] > out[j] {
			out[j-1], out[j] = out[j], out[j-1]
			j--
		}
	}
	return out
}

// EndpointSummaries builds one SummaryRow per endpoint (round=nil, rounds_included=n).
func EndpointSummaries(runs []RunRecord) []SummaryRow {
	nonWarmup := FilterNonWarmup(runs)
	grouped := GroupByEndpoint(nonWarmup)
	var rows []SummaryRow
	for ep, epRuns := range grouped {
		label := ""
		if len(epRuns) > 0 {
			label = epRuns[0].Label
		}
		rows = append(rows, BuildSummaryRow(ep, label, nil, epRuns))
	}
	sortSummaryRows(rows)
	return rows
}

// RoundSummaries builds one SummaryRow per (endpoint, round).
func RoundSummaries(runs []RunRecord) []SummaryRow {
	nonWarmup := FilterNonWarmup(runs)
	type key struct {
		ep    string
		round int
	}
	grouped := map[key][]RunRecord{}
	order := []key{}
	for _, r := range nonWarmup {
		k := key{r.Endpoint, r.Round}
		if _, ok := grouped[k]; !ok {
			order = append(order, k)
		}
		grouped[k] = append(grouped[k], r)
	}
	var rows []SummaryRow
	for _, k := range order {
		epRuns := grouped[k]
		label := ""
		if len(epRuns) > 0 {
			label = epRuns[0].Label
		}
		round := k.round
		rows = append(rows, BuildSummaryRow(k.ep, label, &round, epRuns))
	}
	sortSummaryRows(rows)
	return rows
}

func sortSummaryRows(rows []SummaryRow) {
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && summaryLess(rows[j], rows[j-1]) {
			rows[j-1], rows[j] = rows[j], rows[j-1]
			j--
		}
	}
}

func summaryLess(a, b SummaryRow) bool {
	if a.Endpoint != b.Endpoint {
		return a.Endpoint < b.Endpoint
	}
	ar, br := -1, -1
	if a.Round != nil {
		ar = *a.Round
	}
	if b.Round != nil {
		br = *b.Round
	}
	return ar < br
}
