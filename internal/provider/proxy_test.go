package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIProxyURLApplied(t *testing.T) {
	var gotHost, gotPath, gotAuth string

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.URL.Host
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-proxy",
			"object": "chat.completion",
			"model":  "m",
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       map[string]any{"role": "assistant", "content": "via proxy"},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer proxy.Close()

	p := NewOpenAI(OpenAIConfig{
		BaseURL:  "http://api.openai.invalid/v1",
		APIKey:   "key",
		Model:    "m",
		Timeout:  3 * time.Second,
		ProxyURL: proxy.URL,
	})

	oc, ok := p.(*openAIClient)
	if !ok {
		t.Fatalf("NewOpenAI() type = %T, want *openAIClient", p)
	}
	tr, ok := oc.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", oc.httpClient.Transport)
	}

	// 直接检查 transport 的 Proxy 函数返回配置的代理。
	req, err := http.NewRequest(http.MethodPost, "http://api.openai.invalid/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	gotProxy, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("tr.Proxy(req) error: %v", err)
	}
	if gotProxy == nil || gotProxy.String() != proxy.URL {
		t.Fatalf("tr.Proxy(req) = %v, want %s", gotProxy, proxy.URL)
	}

	// 通过真实 httptest 代理服务器发送请求，确认请求确实走了代理。
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "via proxy" {
		t.Fatalf("Content = %q, want via proxy", resp.Content)
	}
	if gotHost != "api.openai.invalid" {
		t.Fatalf("proxy request host = %q, want api.openai.invalid", gotHost)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("proxy request path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer key" {
		t.Fatalf("proxy request Authorization = %q, want Bearer key", gotAuth)
	}
}

func TestOpenAIProxyFromEnvironmentWired(t *testing.T) {
	// newHTTPClient 在未显式配置 ProxyURL 时使用 http.ProxyFromEnvironment，
	// 从而支持 HTTPS_PROXY / HTTP_PROXY / NO_PROXY 环境变量。
	client := newHTTPClient(time.Second, "")
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", client.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("tr.Proxy = nil, want http.ProxyFromEnvironment")
	}
}
