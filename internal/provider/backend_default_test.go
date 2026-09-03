package provider

import "testing"

func TestDefaultModelDeepSeekIsV4Flash(t *testing.T) {
	if got := defaultModel("deepseek"); got != "deepseek-v4-flash" {
		t.Fatalf("defaultModel(\"deepseek\") = %q, want %q", got, "deepseek-v4-flash")
	}
}

func TestNewProviderDefaultsDeepSeekToV4Flash(t *testing.T) {
	p, err := NewProvider(ProviderOptions{Name: "deepseek"})
	if err != nil {
		t.Fatalf("NewProvider(deepseek) error: %v", err)
	}
	if got := p.Model(); got != "deepseek-v4-flash" {
		t.Fatalf("NewProvider(deepseek).Model() = %q, want %q", got, "deepseek-v4-flash")
	}
}
