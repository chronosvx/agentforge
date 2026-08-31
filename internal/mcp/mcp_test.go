package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHTTPListToolsAndCallTool(t *testing.T) {
	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		writeMCPResponse(t, w, req)
	}
	mux.HandleFunc("/messages", handler)
	mux.HandleFunc("/sse", handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, endpoint := range []string{"/messages", "/sse"} {
		t.Run(endpoint, func(t *testing.T) {
			cl := New(srv.URL + endpoint)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if err := cl.Initialize(ctx); err != nil {
				t.Fatalf("initialize: %v", err)
			}

			tools, err := cl.ListTools(ctx)
			if err != nil {
				t.Fatalf("list tools: %v", err)
			}
			if len(tools) != 1 || tools[0].Name != "echo" || tools[0].Description != "Echo" {
				t.Fatalf("unexpected tools: %#v", tools)
			}
			if tools[0].InputSchema == nil {
				t.Fatalf("expected inputSchema, got nil")
			}

			result, err := cl.CallTool(ctx, "echo", map[string]any{"text": "hello"})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			text, ok := resultText(result)
			if !ok || text != "hello" {
				t.Fatalf("unexpected call result: %#v", result)
			}
		})
	}
}

func TestHTTPSSEResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		var req Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		resp := NewResponse(req.ID, map[string]any{
			"tools": []Tool{{Name: "sse-tool", Description: "SSE tool", InputSchema: map[string]any{"type": "object"}}},
		})
		data, _ := resp.Encode()
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cl := New(srv.URL + "/messages")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tools, err := cl.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools over SSE: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "sse-tool" {
		t.Fatalf("unexpected tools: %#v", tools)
	}
}

func TestHTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		var req Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		writeJSON(t, w, NewErrorResponse(req.ID, -32602, "invalid params", nil))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cl := New(srv.URL + "/messages")
	_, err := cl.ListTools(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "-32602") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStdioClient(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "mock_mcp.go")
	if err := os.WriteFile(src, []byte(stdioMockSource), 0o600); err != nil {
		t.Fatalf("write mock source: %v", err)
	}
	bin := filepath.Join(dir, "mock_mcp")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stdio mock: %v\n%s", err, out)
	}

	cl := New(bin)
	defer func() {
		if err := cl.Close(); err != nil {
			t.Errorf("close stdio client: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := cl.Initialize(ctx); err != nil {
		t.Fatalf("stdio initialize: %v", err)
	}

	tools, err := cl.ListTools(ctx)
	if err != nil {
		t.Fatalf("stdio list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected stdio tools: %#v", tools)
	}

	result, err := cl.CallTool(ctx, "echo", map[string]any{"text": "from stdio"})
	if err != nil {
		t.Fatalf("stdio call tool: %v", err)
	}
	text, ok := resultText(result)
	if !ok || text != "from stdio" {
		t.Fatalf("unexpected stdio call result: %#v", result)
	}
}

func writeMCPResponse(t *testing.T, w http.ResponseWriter, req Request) {
	t.Helper()
	switch req.Method {
	case "initialize":
		writeJSON(t, w, NewResponse(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo":      map[string]any{"name": "mock", "version": "1.0"},
		}))
	case "tools/list":
		writeJSON(t, w, NewResponse(req.ID, map[string]any{
			"tools": []Tool{
				{
					Name:        "echo",
					Description: "Echo",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text": map[string]any{"type": "string"},
						},
					},
				},
			},
		}))
	case "tools/call":
		writeJSON(t, w, NewResponse(req.ID, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "hello"},
			},
			"isError": false,
		}))
	default:
		writeJSON(t, w, NewErrorResponse(req.ID, -32601, "method not found", nil))
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("write json: %v", err)
	}
}

func resultText(result any) (string, bool) {
	m, ok := result.(map[string]any)
	if !ok {
		return "", false
	}
	content, ok := m["content"].([]any)
	if !ok {
		return "", false
	}
	if len(content) == 0 {
		return "", false
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		return "", false
	}
	text, ok := first["text"].(string)
	return text, ok
}

const stdioMockSource = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	ID     any    ` + "`json:\"id\"`" + `
	Method string ` + "`json:\"method\"`" + `
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "mock", "version": "1.0"},
			}
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
						},
					},
				},
			}
		case "tools/call":
			result = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "from stdio"},
				},
				"isError": false,
			}
		default:
			result = map[string]any{}
		}

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}
		data, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		fmt.Println(string(data))
	}
}
`
