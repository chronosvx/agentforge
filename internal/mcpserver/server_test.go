package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/mcp"
	"github.com/chronosvx/agentforge/internal/mcpserver"
)

func TestHandleInitializeToolsListAndCall(t *testing.T) {
	srv := mcpserver.New()
	if err := srv.RegisterTool("echo", func(args map[string]any) (any, error) {
		return args["text"], nil
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	ctx := context.Background()

	// initialize
	initResp := handleJSON(t, srv, ctx, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error)
	}
	var initResult struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      map[string]any `json:"serverInfo"`
	}
	if err := initResp.UnmarshalResult(&initResult); err != nil {
		t.Fatalf("initialize result: %v", err)
	}
	if initResult.ProtocolVersion == "" {
		t.Fatal("expected protocolVersion")
	}
	if initResult.ServerInfo["name"] != "agentforge" {
		t.Fatalf("unexpected serverInfo: %#v", initResult.ServerInfo)
	}

	// tools/list
	listResp := handleJSON(t, srv, ctx, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %v", listResp.Error)
	}
	var listResult struct {
		Tools []mcp.Tool `json:"tools"`
	}
	if err := listResp.UnmarshalResult(&listResult); err != nil {
		t.Fatalf("tools/list result: %v", err)
	}
	if len(listResult.Tools) != 1 || listResult.Tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", listResult.Tools)
	}
	if listResult.Tools[0].InputSchema == nil {
		t.Fatal("expected inputSchema")
	}

	// tools/call
	callResp := handleJSON(t, srv, ctx, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hello"}}}`)
	if callResp.Error != nil {
		t.Fatalf("tools/call error: %v", callResp.Error)
	}
	var callResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := callResp.UnmarshalResult(&callResult); err != nil {
		t.Fatalf("tools/call result: %v", err)
	}
	if callResult.IsError || len(callResult.Content) != 1 || callResult.Content[0].Text != "hello" {
		t.Fatalf("unexpected call result: %#v", callResult)
	}
}

func TestHandleUnknownToolReturnsJSONRPCError(t *testing.T) {
	srv := mcpserver.New()
	resp := handleJSON(t, srv, context.Background(), `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"missing"}}`)
	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("unexpected error code: %d", resp.Error.Code)
	}
}

func TestHandleToolErrorReturnsIsErrorResult(t *testing.T) {
	srv := mcpserver.New()
	if err := srv.RegisterTool("boom", func(args map[string]any) (any, error) {
		return nil, fmt.Errorf("boom failed")
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	resp := handleJSON(t, srv, context.Background(), `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"boom"}}`)
	if resp.Error != nil {
		t.Fatalf("handler error should not be a JSON-RPC error, got %v", resp.Error)
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError || len(result.Content) != 1 || result.Content[0].Text != "boom failed" {
		t.Fatalf("unexpected error result: %#v", result)
	}
}

func TestHandleInvalidJSONReturnsParseError(t *testing.T) {
	srv := mcpserver.New()
	resp := handleJSON(t, srv, context.Background(), `{not-json`)
	if resp.Error == nil || resp.Error.Code != -32700 {
		t.Fatalf("expected parse error, got %#v", resp.Error)
	}
}

func TestServeHTTP(t *testing.T) {
	srv := mcpserver.New()
	if err := srv.RegisterTool("add", func(args map[string]any) (any, error) {
		a, _ := args["a"].(float64)
		b, _ := args["b"].(float64)
		return a + b, nil
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	resp, err := http.Post(httpSrv.URL, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}}}`))
	if err != nil {
		t.Fatalf("http post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var rpcResp mcp.Response
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rpcResp.Error != nil {
		t.Fatalf("unexpected rpc error: %v", rpcResp.Error)
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := rpcResp.UnmarshalResult(&result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "5" {
		t.Fatalf("unexpected add result: %#v", result)
	}
}

func TestServeHTTPMethodNotAllowed(t *testing.T) {
	srv := mcpserver.New()
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	resp, err := http.Get(httpSrv.URL)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestServeStdio(t *testing.T) {
	srv := mcpserver.New()
	if err := srv.RegisterTool("ping", func(args map[string]any) (any, error) {
		return "pong", nil
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ping"}}
`)
	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("serve stdio: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d: %q", len(lines), out.String())
	}
	for _, line := range lines {
		var resp mcp.Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("decode stdio response %q: %v", line, err)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected stdio error: %v", resp.Error)
		}
	}
}

func handleJSON(t *testing.T, srv *mcpserver.Server, ctx context.Context, data string) *mcp.Response {
	t.Helper()
	out := srv.Handle(ctx, []byte(data))
	if len(out) == 0 {
		t.Fatal("expected response bytes")
	}
	resp, err := mcp.DecodeResponse(out)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}
