package provider

import (
	"encoding/json"
	"testing"
)

func TestToolCallJSONRoundTrip(t *testing.T) {
	in := []ToolCall{
		{
			ID:        "call_1",
			Type:      "function",
			Name:      "get_weather",
			Arguments: `{"city":"Beijing"}`,
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal ToolCall: %v", err)
	}

	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw ToolCall: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("len(raw) = %d, want 1", len(raw))
	}
	fn, ok := raw[0]["function"].(map[string]any)
	if !ok {
		t.Fatalf("raw[0][\"function\"] = %#v, want map", raw[0]["function"])
	}
	if fn["name"] != "get_weather" {
		t.Fatalf("function.name = %v, want get_weather", fn["name"])
	}
	if fn["arguments"] != `{"city":"Beijing"}` {
		t.Fatalf("function.arguments = %v, want JSON arguments", fn["arguments"])
	}

	var got []ToolCall
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal ToolCall: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0] != in[0] {
		t.Fatalf("round trip = %#v, want %#v", got[0], in[0])
	}
}

func TestMessageToolCallsJSON(t *testing.T) {
	msg := Message{
		Role: "assistant",
		ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Name: "lookup", Arguments: `{"key":"a"}`},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal Message: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw Message: %v", err)
	}
	calls, ok := raw["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls = %#v, want one call", raw["tool_calls"])
	}
	call, ok := calls[0].(map[string]any)
	if !ok {
		t.Fatalf("tool_calls[0] = %#v, want map", calls[0])
	}
	fn, ok := call["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool_calls[0].function = %#v, want map", call["function"])
	}
	if fn["name"] != "lookup" {
		t.Fatalf("function.name = %v, want lookup", fn["name"])
	}
}

func TestToolCallDefaultsTypeToFunction(t *testing.T) {
	data, err := json.Marshal(ToolCall{ID: "call_1", Name: "f", Arguments: "{}"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if raw["type"] != "function" {
		t.Fatalf("type = %v, want function", raw["type"])
	}
}
