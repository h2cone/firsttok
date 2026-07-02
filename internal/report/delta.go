package report

import (
	"regexp"
	"sort"

	"github.com/firsttok/firsttok/internal/stats"
)

// DeltaRow is one per-round left-minus-right delta entry.
type DeltaRow struct {
	Round         *int
	Comparison    string
	Meaning       string
	LeftEndpoint  string
	RightEndpoint string
	P50Delta      *float64
	P90Delta      *float64
	P95Delta      *float64
	AvgDelta      *float64
	MinDelta      *float64
	MaxDelta      *float64
}

// DeltaSummaryRow is the pooled/per-round aggregation for one comparison.
type DeltaSummaryRow struct {
	Comparison          string
	Meaning             string
	RoundsCompared      int
	PooledP50Delta      *float64
	PooledP95Delta      *float64
	PooledAvgDelta      *float64
	RoundP50DeltaMedian *float64
	RoundP50DeltaMin    *float64
	RoundP50DeltaMax    *float64
	RoundP95DeltaMedian *float64
	RoundAvgDeltaMedian *float64
}

var unsafeCompChar = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func normalizeComparisonName(value string) string {
	s := unsafeCompChar.ReplaceAllString(value, "_")
	s = trimUnderscore(s)
	if s == "" {
		return "comparison"
	}
	return s
}

func trimUnderscore(s string) string {
	for len(s) > 0 && s[0] == '_' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '_' {
		s = s[:len(s)-1]
	}
	return s
}

// BuildComparisons constructs the comparison list from the ordered targets.
// Business comparisons are added when dmxapi/dcs/dcs-no-plugin-proxy are all
// present; then auto pairwise comparisons are appended.
func BuildComparisons(targets []Target) []Comparison {
	keys := map[string]bool{}
	for _, t := range targets {
		keys[t.Key] = true
	}
	var comps []Comparison
	if keys["dmxapi"] && keys["dcs"] && keys["dcs-no-plugin-proxy"] {
		comps = append(comps,
			Comparison{Name: "dcs_vs_dmxapi", Meaning: "DCS plugin+proxy path minus DMXAPI direct", Left: "dcs", Right: "dmxapi"},
			Comparison{Name: "dcs_no_plugin_proxy_vs_dmxapi", Meaning: "DCS base path minus DMXAPI direct", Left: "dcs-no-plugin-proxy", Right: "dmxapi"},
			Comparison{Name: "plugin_solution_overhead", Meaning: "DCS plugin+proxy path minus DCS no-plugin/proxy path", Left: "dcs", Right: "dcs-no-plugin-proxy"},
		)
	}
	comparedPairs := map[string]bool{}
	for i := 1; i < len(targets); i++ {
		for j := 0; j < i; j++ {
			pair := pairKey(targets[i].Key, targets[j].Key)
			if comparedPairs[pair] {
				continue
			}
			comparedPairs[pair] = true
			comps = append(comps, Comparison{
				Name:    normalizeComparisonName(targets[i].Key + "_vs_" + targets[j].Key),
				Meaning: targets[i].Label + " minus " + targets[j].Label,
				Left:    targets[i].Key,
				Right:   targets[j].Key,
			})
		}
	}
	return comps
}

func pairKey(a, b string) string {
	if a < b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}

func findSummary(rows []SummaryRow, endpoint string, round *int) *SummaryRow {
	for i := range rows {
		if rows[i].Endpoint != endpoint {
			continue
		}
		if round == nil && rows[i].Round == nil {
			return &rows[i]
		}
		if round != nil && rows[i].Round != nil && *rows[i].Round == *round {
			return &rows[i]
		}
	}
	return nil
}

func newDeltaRow(left, right *SummaryRow, comp Comparison, round *int) *DeltaRow {
	if left == nil || right == nil {
		return nil
	}
	r := round
	if r == nil {
		if left.Round != nil {
			lr := *left.Round
			r = &lr
		}
	}
	return &DeltaRow{
		Round:         r,
		Comparison:    comp.Name,
		Meaning:       comp.Meaning,
		LeftEndpoint:  left.Endpoint,
		RightEndpoint: right.Endpoint,
		P50Delta:      stats.SubtractNullable(left.TTFTP50, right.TTFTP50),
		P90Delta:      stats.SubtractNullable(left.TTFTP90, right.TTFTP90),
		P95Delta:      stats.SubtractNullable(left.TTFTP95, right.TTFTP95),
		AvgDelta:      stats.SubtractNullable(left.TTFTAvg, right.TTFTAvg),
		MinDelta:      stats.SubtractNullable(left.TTFTMin, right.TTFTMin),
		MaxDelta:      stats.SubtractNullable(left.TTFTMax, right.TTFTMax),
	}
}

// DeltaByRound builds per-round delta rows from round summaries.
func DeltaByRound(roundSummaries []SummaryRow, comps []Comparison, rounds []int) []DeltaRow {
	var rows []DeltaRow
	for _, rn := range rounds {
		r := rn
		for _, c := range comps {
			left := findSummary(roundSummaries, c.Left, &r)
			right := findSummary(roundSummaries, c.Right, &r)
			if dr := newDeltaRow(left, right, c, &r); dr != nil {
				rows = append(rows, *dr)
			}
		}
	}
	return rows
}

// DeltaSummary builds the pooled delta summary rows.
func DeltaSummary(endpointSummaries []SummaryRow, byRound []DeltaRow, comps []Comparison) []DeltaSummaryRow {
	var out []DeltaSummaryRow
	for _, c := range comps {
		pooledLeft := findSummary(endpointSummaries, c.Left, nil)
		pooledRight := findSummary(endpointSummaries, c.Right, nil)
		row := DeltaSummaryRow{Comparison: c.Name, Meaning: c.Meaning}
		var p50, p95, avg []float64
		for _, dr := range byRound {
			if dr.Comparison != c.Name {
				continue
			}
			row.RoundsCompared++
			if dr.P50Delta != nil {
				p50 = append(p50, *dr.P50Delta)
			}
			if dr.P95Delta != nil {
				p95 = append(p95, *dr.P95Delta)
			}
			if dr.AvgDelta != nil {
				avg = append(avg, *dr.AvgDelta)
			}
		}
		if pooledLeft != nil && pooledRight != nil {
			row.PooledP50Delta = stats.SubtractNullable(pooledLeft.TTFTP50, pooledRight.TTFTP50)
			row.PooledP95Delta = stats.SubtractNullable(pooledLeft.TTFTP95, pooledRight.TTFTP95)
			row.PooledAvgDelta = stats.SubtractNullable(pooledLeft.TTFTAvg, pooledRight.TTFTAvg)
		}
		row.RoundP50DeltaMedian = stats.RoundNullable(stats.Percentile(p50, 0.50))
		row.RoundP50DeltaMin = stats.RoundNullable(stats.Min(p50))
		row.RoundP50DeltaMax = stats.RoundNullable(stats.Max(p50))
		row.RoundP95DeltaMedian = stats.RoundNullable(stats.Percentile(p95, 0.50))
		row.RoundAvgDeltaMedian = stats.RoundNullable(stats.Percentile(avg, 0.50))
		out = append(out, row)
	}
	return out
}

// SortDeltaRows orders delta_by_round rows by round then comparison name.
func SortDeltaRows(rows []DeltaRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := -1, -1
		if rows[i].Round != nil {
			ri = *rows[i].Round
		}
		if rows[j].Round != nil {
			rj = *rows[j].Round
		}
		if ri != rj {
			return ri < rj
		}
		return rows[i].Comparison < rows[j].Comparison
	})
}
