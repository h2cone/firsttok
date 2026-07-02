// Package stats implements the statistics used by aggregation: linear-interp
// percentiles, sample standard deviation, rounding, and summary builders.
package stats

import (
	"math"
	"sort"
)

// Percentile returns the p-quantile (0..1) using linear interpolation:
// rank = (n-1) * p. Returns nil for empty input.
func Percentile(values []float64, p float64) *float64 {
	items := sortedCopy(values)
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		v := items[0]
		return &v
	}
	rank := float64(len(items)-1) * p
	lower := int(math.Floor(rank))
	upper := lower + 1
	if upper > len(items)-1 {
		upper = len(items) - 1
	}
	weight := rank - float64(lower)
	v := items[lower]*(1.0-weight) + items[upper]*weight
	return &v
}

// StdDev returns the sample standard deviation (n-1 denominator); nil for n<=1.
func StdDev(values []float64) *float64 {
	n := len(values)
	if n <= 1 {
		return nil
	}
	avg := Mean(values)
	var sumSq float64
	for _, v := range values {
		d := v - avg
		sumSq += d * d
	}
	sd := math.Sqrt(sumSq / float64(n-1))
	return &sd
}

// Mean returns the arithmetic mean, or 0 for empty input.
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// Min returns the minimum, or nil for empty input.
func Min(values []float64) *float64 {
	items := sortedCopy(values)
	if len(items) == 0 {
		return nil
	}
	v := items[0]
	return &v
}

// Max returns the maximum, or nil for empty input.
func Max(values []float64) *float64 {
	items := sortedCopy(values)
	if len(items) == 0 {
		return nil
	}
	v := items[len(items)-1]
	return &v
}

// RoundNullable rounds v to 3 decimals, normalizing -0.0 to 0.0. nil stays nil.
func RoundNullable(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := math.Round(*v*1000) / 1000
	if r == 0 {
		r = 0.0
	}
	return &r
}

// SubtractNullable returns left-right rounded to 3 decimals, nil if either nil.
func SubtractNullable(left, right *float64) *float64 {
	if left == nil || right == nil {
		return nil
	}
	d := *left - *right
	return RoundNullable(&d)
}

func sortedCopy(values []float64) []float64 {
	out := make([]float64, len(values))
	copy(out, values)
	sort.Float64s(out)
	return out
}
