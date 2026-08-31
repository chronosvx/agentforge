package provider

import (
	"errors"
	"testing"
)

func TestToOpenAIMessagesSystemUserAssistantTool(t *testing.T) {
	msgs := []providerMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "", Name: "get_weather", Arguments: `{"city":"Beijing"}`},
			},
		},
		{Role: "tool", Content: `{"temp":26}`, ToolCallID: "call_1"},
	}

	got := ToOpenAIMessages(msgs)
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}

	if got[0]["role"] != "system" || got[0]["content"] != "You are helpful." {
		t.Fatalf("system message = %#v", got[0])
	}
	if got[1]["role"] != "user" || got[1]["content"] != "Hello" {
		t.Fatalf("user message = %#v", got[1])
	}

	assistant := got[2]
	if assistant["role"] != "assistant" {
		t.Fatalf("assistant role = %#v", assistant["role"])
	}
	calls, ok := assistant["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("assistant tool_calls = %#v, want one call", assistant["tool_calls"])
	}
	call := calls[0]
	if call["id"] != "call_1" || call["type"] != "function" {
		t.Fatalf("tool call = %#v", call)
	}
	fn, ok := call["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool call function = %#v, want map", call["function"])
	}
	if fn["name"] != "get_weather" || fn["arguments"] != `{"city":"Beijing"}` {
		t.Fatalf("tool call function = %#v", fn)
	}

	tool := got[3]
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_1" || tool["content"] != `{"temp":26}` {
		t.Fatalf("tool message = %#v", tool)
	}
}

func TestToOpenAIMessagesEmpty(t *testing.T) {
	got := ToOpenAIMessages(nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("ToOpenAIMessages(nil) = %#v, want empty non-nil slice", got)
	}
}

func TestConvertSessionForProviderOpenAICompatible(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "hi"},
	}

	for _, provider := range []string{"openai", "deepseek", "moonshot", "kimi", "zhipu", "qwen", "ollama", " OpenAI "} {
		got, err := ConvertSessionForProvider(provider, messages)
		if err != nil {
			t.Fatalf("ConvertSessionForProvider(%q) error: %v", provider, err)
		}
		if len(got) != 1 || got[0]["content"] != "hi" {
			t.Fatalf("ConvertSessionForProvider(%q) = %#v", provider, got)
		}
	}
}

func TestConvertSessionForProviderUnsupported(t *testing.T) {
	_, err := ConvertSessionForProvider("anthropic", []map[string]any{{"role": "user", "content": "hi"}})
	if err == nil {
		t.Fatal("ConvertSessionForProvider(anthropic) error = nil, want error")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("error = %v, want ErrUnsupportedProvider", err)
	}
}
