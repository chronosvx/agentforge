package cost

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestEstimateFormula(t *testing.T) {
	e := NewCustomEstimator(2, 3, 0.5, 1)

	got := e.Estimate(1_000_000, 500_000, 200_000, 100_000)
	want := 2.0 + 1.5 + 0.1 + 0.1
	if !almostEqual(got, want) {
		t.Fatalf("Estimate() = %v, want %v", got, want)
	}
}

func TestEstimateZeroTokens(t *testing.T) {
	e := NewEstimator()
	if got := e.Estimate(0, 0, 0, 0); got != 0 {
		t.Fatalf("Estimate() with zero tokens = %v, want 0", got)
	}
}

func TestEstimateFromUsage(t *testing.T) {
	e := NewCustomEstimator(1, 2, 0.5, 0.25)
	usage := map[string]any{
		"uncachedInputTokens": float64(1000),
		"outputTokens":        float64(2000),
		"cacheReadTokens":     float64(4000),
		"cacheWriteTokens":    float64(8000),
	}

	got := e.EstimateFromUsage(usage)
	want := 1000*1/1e6 + 2000*2/1e6 + 4000*0.5/1e6 + 8000*0.25/1e6
	if !almostEqual(got, want) {
		t.Fatalf("EstimateFromUsage() = %v, want %v", got, want)
	}
}

func TestEstimateFromUsageEmptyMap(t *testing.T) {
	e := NewEstimator()
	if got := e.EstimateFromUsage(map[string]any{}); got != 0 {
		t.Fatalf("EstimateFromUsage(empty) = %v, want 0", got)
	}
	if got := EstimateFromUsage(nil); got != 0 {
		t.Fatalf("EstimateFromUsage(nil) = %v, want 0", got)
	}
}

func TestEstimateFromUsageAliases(t *testing.T) {
	e := NewCustomEstimator(1, 2, 0.5, 0.25)
	usage := map[string]any{
		"input":      float64(1000),
		"output":     float64(2000),
		"cacheHits":  float64(4000),
		"cacheWrite": float64(8000),
	}

	got := e.EstimateFromUsage(usage)
	want := 1000*1/1e6 + 2000*2/1e6 + 4000*0.5/1e6 + 8000*0.25/1e6
	if !almostEqual(got, want) {
		t.Fatalf("EstimateFromUsage(aliases) = %v, want %v", got, want)
	}
}

func TestBudgetAddAndRemaining(t *testing.T) {
	b := NewBudget(10)
	if b.Remaining != 10 {
		t.Fatalf("initial Remaining = %v, want 10", b.Remaining)
	}
	if b.Exceeded() {
		t.Fatal("new budget should not be exceeded")
	}

	b.Add(4)
	if b.Remaining != 6 {
		t.Fatalf("Remaining after Add(4) = %v, want 6", b.Remaining)
	}
	if b.Exceeded() {
		t.Fatal("budget should not be exceeded at 4/10")
	}

	b.Add(6)
	if b.Remaining != 0 {
		t.Fatalf("Remaining after Add(6) = %v, want 0", b.Remaining)
	}
	if b.Exceeded() {
		t.Fatal("budget should not be exceeded exactly at 10/10")
	}

	b.Add(0.01)
	if b.Remaining >= 0 {
		t.Fatalf("Remaining after overrun = %v, want negative", b.Remaining)
	}
	if !b.Exceeded() {
		t.Fatal("budget should be exceeded after overrun")
	}
}
