package provider

import "testing"

func TestDefaultModelRegistryHasCommonModels(t *testing.T) {
	reg := DefaultModelRegistry()

	cases := []struct {
		name        string
		wantContext int64
		wantMaxOut  int64
		wantTools   bool
		wantStream  bool
	}{
		{name: "DeepSeek-chat", wantContext: 64 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "deepseek-reasoner", wantContext: 64 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "gpt-4o", wantContext: 128 * 1024, wantMaxOut: 16 * 1024, wantTools: true, wantStream: true},
		{name: "gpt-4o-mini", wantContext: 128 * 1024, wantMaxOut: 16 * 1024, wantTools: true, wantStream: true},
		{name: "kimi-k2", wantContext: 128 * 1024, wantMaxOut: 32 * 1024, wantTools: true, wantStream: true},
		{name: "glm-4-plus", wantContext: 128 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "glm-4-flash", wantContext: 128 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "qwen-max", wantContext: 32 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "qwen-plus", wantContext: 128 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "qwen2.5:7b", wantContext: 32 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "qwen2.5:14b", wantContext: 32 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "llama3.1:8b", wantContext: 128 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "llama3.2:3b", wantContext: 128 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
		{name: "deepseek-r1:7b", wantContext: 64 * 1024, wantMaxOut: 8 * 1024, wantTools: true, wantStream: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, ok := reg.Get(tc.name)
			if !ok {
				t.Fatalf("Get(%q) ok = false, want true", tc.name)
			}
			if info.Name != tc.name {
				t.Fatalf("Get(%q).Name = %q, want %q", tc.name, info.Name, tc.name)
			}
			if info.ContextWindow != tc.wantContext {
				t.Fatalf("Get(%q).ContextWindow = %d, want %d", tc.name, info.ContextWindow, tc.wantContext)
			}
			if info.MaxOutput != tc.wantMaxOut {
				t.Fatalf("Get(%q).MaxOutput = %d, want %d", tc.name, info.MaxOutput, tc.wantMaxOut)
			}
			if info.SupportsTools != tc.wantTools {
				t.Fatalf("Get(%q).SupportsTools = %v, want %v", tc.name, info.SupportsTools, tc.wantTools)
			}
			if info.SupportsStreaming != tc.wantStream {
				t.Fatalf("Get(%q).SupportsStreaming = %v, want %v", tc.name, info.SupportsStreaming, tc.wantStream)
			}
		})
	}
}

func TestGetUnknownFallsBackToDefault(t *testing.T) {
	reg := DefaultModelRegistry()

	info, ok := reg.Get("unknown-model")
	if ok {
		t.Fatalf("Get(unknown) ok = true, want false")
	}
	if info != DefaultModelInfo() {
		t.Fatalf("Get(unknown) = %+v, want DefaultModelInfo() = %+v", info, DefaultModelInfo())
	}
}

func TestGetNilRegistryFallsBackToDefault(t *testing.T) {
	var reg *ModelRegistry

	info, ok := reg.Get("gpt-4o")
	if ok {
		t.Fatalf("Get on nil registry ok = true, want false")
	}
	if info != DefaultModelInfo() {
		t.Fatalf("Get on nil registry = %+v, want DefaultModelInfo()", info)
	}
}

func TestDefaultReturnsDeepSeekChat(t *testing.T) {
	reg := NewModelRegistry()
	if got := reg.Default(); got != DefaultModelInfo() {
		t.Fatalf("Default() = %+v, want DefaultModelInfo() = %+v", got, DefaultModelInfo())
	}
	if got := reg.Default().Name; got != "DeepSeek-chat" {
		t.Fatalf("Default().Name = %q, want DeepSeek-chat", got)
	}
}

func TestModelRegistryRegisterOverridesAndNormalizesKey(t *testing.T) {
	reg := NewModelRegistry()
	want := ModelInfo{
		Name:              "My-Model",
		ContextWindow:     123456,
		MaxOutput:         4096,
		SupportsTools:     false,
		SupportsStreaming: true,
	}
	reg.Register(want)

	got, ok := reg.Get("  my-model ")
	if !ok {
		t.Fatalf("Get(normalized) ok = false, want true")
	}
	if got != want {
		t.Fatalf("Get(normalized) = %+v, want %+v", got, want)
	}

	override := want
	override.ContextWindow = 999999
	reg.Register(override)
	got, ok = reg.Get("MY-MODEL")
	if !ok {
		t.Fatalf("Get(overridden) ok = false, want true")
	}
	if got.ContextWindow != 999999 {
		t.Fatalf("Get(overridden).ContextWindow = %d, want 999999", got.ContextWindow)
	}
}
