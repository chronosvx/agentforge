package cost_test

import (
	"math"
	"testing"

	"github.com/chronosvx/agentforge/internal/cost"
	"github.com/chronosvx/agentforge/internal/provider"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestEstimateFromUsageWithPricingPayAsYouGo(t *testing.T) {
	pricing := provider.Pricing{
		Provider: "Test", Model: "model",
		InputPerMillion: 2, OutputPerMillion: 3,
		CacheReadPerMillion: 0.5, CacheWritePerMillion: 1,
		Currency: "CNY", PlanType: provider.PlanTypePayAsYouGo,
	}
	usage := map[string]any{
		"uncachedInputTokens": float64(1_000_000),
		"outputTokens":        float64(500_000),
		"cacheReadTokens":     float64(200_000),
		"cacheWriteTokens":    float64(100_000),
	}

	got := cost.EstimateFromUsageWithPricing(usage, pricing)
	want := 2 + 1.5 + 0.1 + 0.1
	if !almostEqual(got, want) {
		t.Fatalf("EstimateFromUsageWithPricing() = %v, want %v", got, want)
	}
}

func TestEstimateFromUsageWithPricingTokenPlan(t *testing.T) {
	pricing := provider.Pricing{
		Provider: "Test", Model: "plan",
		Currency: "CNY", PlanType: provider.PlanTypeTokenPlan, MonthlyFee: 199,
	}
	usage := map[string]any{
		"uncachedInputTokens": float64(99_000_000),
		"outputTokens":        float64(88_000_000),
	}

	got := cost.EstimateFromUsageWithPricing(usage, pricing)
	if !almostEqual(got, 199) {
		t.Fatalf("EstimateFromUsageWithPricing(token plan) = %v, want 199", got)
	}
}

func TestEstimateFromUsageWithPricingDefaultFallback(t *testing.T) {
	table := provider.DefaultPricingTable()
	pricing, ok := table.Get("NoSuchProvider", "no-such-model")
	if ok {
		t.Fatalf("Get(unknown) ok = true, want false")
	}

	usage := map[string]any{
		"uncachedInputTokens": float64(1_000_000),
		"outputTokens":        float64(500_000),
		"cacheReadTokens":     float64(200_000),
		"cacheWriteTokens":    float64(100_000),
	}

	got := cost.EstimateFromUsageWithPricing(usage, pricing)
	want := 1.5 + 2.25 + 0.01 + 0.15
	if !almostEqual(got, want) {
		t.Fatalf("EstimateFromUsageWithPricing(default fallback) = %v, want %v", got, want)
	}
}

func TestEstimateFromUsageWithPricingEmptyUsage(t *testing.T) {
	pricing := provider.DefaultPricing()
	if got := cost.EstimateFromUsageWithPricing(nil, pricing); got != 0 {
		t.Fatalf("EstimateFromUsageWithPricing(nil) = %v, want 0", got)
	}
}
