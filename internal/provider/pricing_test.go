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
		{"DeepSeek", "deepseek-v4-flash"},
		{"DeepSeek", "deepseek-v4-pro"},
		{"DeepSeek", "deepseek-v4-vision"},
		{"DeepSeek", "deepseek-v4-flash-vision-exp"},
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

func TestDeepSeekV4PricingValues(t *testing.T) {
	table := DefaultPricingTable()

	cases := []struct {
		model      string
		input      float64
		output     float64
		cacheRead  float64
		cacheWrite float64
	}{
		{model: "deepseek-v4-flash", input: 1.0, output: 2.0, cacheRead: 0.05, cacheWrite: 1.0},
		{model: "deepseek-v4-flash-vision-exp", input: 1.0, output: 2.0, cacheRead: 0.05, cacheWrite: 1.0},
		{model: "deepseek-v4-pro", input: 4.0, output: 16.0, cacheRead: 1.0, cacheWrite: 4.0},
		{model: "deepseek-v4-vision", input: 2.0, output: 8.0, cacheRead: 0.5, cacheWrite: 2.0},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			p, ok := table.Get("DeepSeek", tc.model)
			if !ok {
				t.Fatalf("Get(%q, %q) ok = false, want true", "DeepSeek", tc.model)
			}
			if p.InputPerMillion != tc.input {
				t.Fatalf("%s InputPerMillion = %v, want %v", tc.model, p.InputPerMillion, tc.input)
			}
			if p.OutputPerMillion != tc.output {
				t.Fatalf("%s OutputPerMillion = %v, want %v", tc.model, p.OutputPerMillion, tc.output)
			}
			if p.CacheReadPerMillion != tc.cacheRead {
				t.Fatalf("%s CacheReadPerMillion = %v, want %v", tc.model, p.CacheReadPerMillion, tc.cacheRead)
			}
			if p.CacheWritePerMillion != tc.cacheWrite {
				t.Fatalf("%s CacheWritePerMillion = %v, want %v", tc.model, p.CacheWritePerMillion, tc.cacheWrite)
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
