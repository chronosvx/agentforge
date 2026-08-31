package provider

import (
	"testing"
)

func TestDefaultPricingTableHasCommonModels(t *testing.T) {
	table := DefaultPricingTable()

	cases := []struct {
		provider string
		model    string
	}{
		{"DeepSeek", "DeepSeek-chat"},
		{"DeepSeek", "deepseek-reasoner"},
		{"OpenAI", "gpt-4o"},
		{"OpenAI", "gpt-4o-mini"},
		{"Moonshot", "kimi-k2"},
		{"Moonshot", "moonshot-v1-8k"},
		{"Zhipu", "glm-4-plus"},
		{"Zhipu", "glm-4-flash"},
		{"Qwen", "qwen-max"},
		{"Qwen", "qwen-plus"},
		{"Ollama", "llama3"},
		{"Ollama", "qwen2.5"},
		{"Ollama", "deepseek-r1"},
	}

	for _, tc := range cases {
		t.Run(tc.provider+"/"+tc.model, func(t *testing.T) {
			p, ok := table.Get(tc.provider, tc.model)
			if !ok {
				t.Fatalf("Get(%q, %q) ok = false, want true", tc.provider, tc.model)
			}
			if p.Provider != tc.provider || p.Model != tc.model {
				t.Fatalf("Get(%q, %q) = %+v, want matching provider/model", tc.provider, tc.model, p)
			}
		})
	}
}

func TestGetUnknownFallsBackToDeepSeek(t *testing.T) {
	table := DefaultPricingTable()

	p, ok := table.Get("UnknownProvider", "unknown-model")
	if ok {
		t.Fatalf("Get(unknown) ok = true, want false")
	}
	if p != DefaultPricing() {
		t.Fatalf("Get(unknown) = %+v, want DefaultPricing() = %+v", p, DefaultPricing())
	}
}

func TestGetNilTableFallsBackToDeepSeek(t *testing.T) {
	var table *PricingTable

	p, ok := table.Get("DeepSeek", "DeepSeek-chat")
	if ok {
		t.Fatalf("Get on nil table ok = true, want false")
	}
	if p != DefaultPricing() {
		t.Fatalf("Get on nil table = %+v, want DefaultPricing()", p)
	}
}

func TestRegisterOverridesAndNormalizesKey(t *testing.T) {
	table := NewPricingTable()
	want := Pricing{
		Provider: "MyProvider", Model: "my-model",
		InputPerMillion: 2, OutputPerMillion: 3,
		CacheReadPerMillion: 0.5, CacheWritePerMillion: 1,
		Currency: "USD", PlanType: PlanTypePayAsYouGo,
	}
	table.Register(want)

	got, ok := table.Get("  myprovider ", "MY-MODEL")
	if !ok {
		t.Fatalf("Get(normalized) ok = false, want true")
	}
	if got != want {
		t.Fatalf("Get(normalized) = %+v, want %+v", got, want)
	}
}

func TestTokenPlanPricingFields(t *testing.T) {
	p := Pricing{
		Provider: "Example", Model: "plan-model",
		Currency: "CNY", PlanType: PlanTypeTokenPlan, MonthlyFee: 199,
	}
	if p.PlanType != PlanTypeTokenPlan {
		t.Fatalf("PlanType = %q, want %q", p.PlanType, PlanTypeTokenPlan)
	}
	if p.MonthlyFee != 199 {
		t.Fatalf("MonthlyFee = %v, want 199", p.MonthlyFee)
	}
}
