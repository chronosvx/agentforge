package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIChatNonStream(t *testing.T) {
	var gotPath, gotAuth, gotContentType string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-1",
			"object": "chat.completion",
			"model":  "req-model",
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       map[string]any{"role": "assistant", "content": "Hello from mock"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
				"prompt_tokens_details": map[string]any{
					"cached_tokens": 3,
				},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "test-key", Model: "cfg-model"})
	temp := 0.7
	resp, err := p.Chat(context.Background(), ChatRequest{
		Model:       "req-model",
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["model"] != "req-model" {
		t.Fatalf("model = %v, want req-model", gotBody["model"])
	}
	if gotBody["stream"] != false {
		t.Fatalf("stream = %v, want false", gotBody["stream"])
	}
	if gotBody["temperature"] != 0.7 {
		t.Fatalf("temperature = %v, want 0.7", gotBody["temperature"])
	}
	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v, want one message", gotBody["messages"])
	}

	if resp.Content != "Hello from mock" {
		t.Fatalf("Content = %q, want Hello from mock", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("FinishReason = %q, want stop", resp.FinishReason)
	}
	if resp.Usage == nil {
		t.Fatal("Usage = nil, want non-nil")
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("Usage = %+v, want input=10 output=5", resp.Usage)
	}
	if resp.Usage.CacheReadTokens != 3 {
		t.Fatalf("CacheReadTokens = %d, want 3", resp.Usage.CacheReadTokens)
	}
}

func TestOpenAIChatToolCall(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-tool",
			"object": "chat.completion",
			"model":  "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_weather",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"Beijing"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "", Model: "test-model"})
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "weather?"}},
		Tools: []Tool{
			{
				Function: ToolFunction{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	tools, ok := gotBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", gotBody["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tools[0] = %#v, want map", tools[0])
	}
	if tool["type"] != "function" {
		t.Fatalf("tool.type = %v, want function", tool["type"])
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %#v, want one call", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_weather" || tc.Name != "get_weather" || tc.Arguments != `{"city":"Beijing"}` {
		t.Fatalf("ToolCall = %#v, want weather call", tc)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q, want tool_calls", resp.FinishReason)
	}
}

func TestOpenAIChatError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "bad", Model: "m"})
	_, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("Chat() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("error = %v, want to contain invalid api key", err)
	}
	if IsRetryable(err) {
		t.Fatalf("IsRetryable(err) = true for auth error, want false")
	}
}

func TestOpenAIStreamChat(t *testing.T) {
	var gotStream any
	var gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotStream = body["stream"]
		gotAccept = r.Header.Get("Accept")

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		write := func(payload string) {
			_, _ = w.Write([]byte("data: " + payload + "\n\n"))
			flusher.Flush()
		}

		write(`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`)
		write(`{"choices":[{"index":0,"delta":{"content":" world"}}]}`)
		write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_weather","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Bei"}}]}}]}`)
		write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"jing\"}"}}]}}]}`)
		write(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		write(`{"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12},"choices":[]}`)
		write(`[DONE]`)
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "key", Model: "m"})
	ch, err := p.StreamChat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("StreamChat() error: %v", err)
	}

	var text strings.Builder
	var toolCall *ToolCall
	var usage *Usage
	done := false

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				if !done {
					t.Fatal("stream closed without done event")
				}
				goto collected
			}
			switch ev.Type {
			case StreamEventText:
				text.WriteString(ev.Text)
			case StreamEventToolCall:
				toolCall = ev.ToolCall
			case StreamEventUsage:
				usage = ev.Usage
			case StreamEventDone:
				done = true
			case StreamEventError:
				t.Fatalf("stream error event: %v", ev.Err)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for stream events")
		}
	}
collected:

	if gotStream != true {
		t.Fatalf("stream = %v, want true", gotStream)
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", gotAccept)
	}
	if text.String() != "Hello world" {
		t.Fatalf("text = %q, want Hello world", text.String())
	}
	if toolCall == nil {
		t.Fatal("toolCall = nil, want tool call event")
	}
	if toolCall.ID != "call_weather" || toolCall.Name != "get_weather" || toolCall.Arguments != `{"city":"Beijing"}` {
		t.Fatalf("toolCall = %#v, want accumulated weather call", toolCall)
	}
	if usage == nil {
		t.Fatal("usage = nil, want usage event")
	}
	if usage.InputTokens != 8 || usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v, want input=8 output=4", usage)
	}
	if !done {
		t.Fatal("done = false, want true")
	}
}

func TestOpenAIStreamChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "key", Model: "m"})
	_, err := p.StreamChat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("StreamChat() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v, want to contain rate limited", err)
	}
	if !IsRetryable(err) {
		t.Fatalf("IsRetryable(err) = false for rate limit, want true")
	}
}
