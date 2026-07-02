package stats

import (
	"math"
	"testing"
)

func nearlyEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPercentile(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5}
	// rank = (5-1)*0.5 = 2 -> items[2] = 3
	p50 := Percentile(v, 0.5)
	if p50 == nil || !nearlyEqual(*p50, 3.0) {
		t.Errorf("p50 = %v", p50)
	}
	// p25: rank=1 -> items[1]=2
	p25 := Percentile(v, 0.25)
	if p25 == nil || !nearlyEqual(*p25, 2.0) {
		t.Errorf("p25 = %v", p25)
	}
	// p0 -> min, p1 -> max
	p0 := Percentile(v, 0.0)
	if p0 == nil || !nearlyEqual(*p0, 1.0) {
		t.Errorf("p0 = %v", p0)
	}
	p100 := Percentile(v, 1.0)
	if p100 == nil || !nearlyEqual(*p100, 5.0) {
		t.Errorf("p100 = %v", p100)
	}
	if Percentile(nil, 0.5) != nil {
		t.Error("empty should be nil")
	}
}

func TestPercentileSingle(t *testing.T) {
	p := Percentile([]float64{42}, 0.5)
	if p == nil || *p != 42 {
		t.Errorf("single = %v", p)
	}
}

func TestStdDev(t *testing.T) {
	v := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	sd := StdDev(v)
	// sample stddev: sum of squared deviations = 32, /7 = 4.5714, sqrt = 2.138.
	want := math.Sqrt(32.0 / 7.0)
	if sd == nil || !nearlyEqual(*sd, want) {
		t.Errorf("stddev = %v, want %v", sd, want)
	}
	if StdDev([]float64{1}) != nil {
		t.Error("single stddev should be nil")
	}
}

func TestRoundNullableNegativeZero(t *testing.T) {
	neg := -0.0
	r := RoundNullable(&neg)
	if *r != 0.0 || math.Signbit(*r) {
		t.Errorf("-0.0 should normalize to +0.0, got %v signbit=%v", *r, math.Signbit(*r))
	}
	// 1.2345 -> 1.235 (half away from zero)
	v := 1.2345
	r2 := RoundNullable(&v)
	if *r2 != 1.235 {
		t.Errorf("round(1.2345) = %v, want 1.235", *r2)
	}
	if RoundNullable(nil) != nil {
		t.Error("nil should stay nil")
	}
}

func TestSubtractNullable(t *testing.T) {
	a, b := 5.0, 2.0
	r := SubtractNullable(&a, &b)
	if *r != 3.0 {
		t.Errorf("subtract = %v", *r)
	}
	if SubtractNullable(nil, &b) != nil {
		t.Error("nil operand should give nil")
	}
}
