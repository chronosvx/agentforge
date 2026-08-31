package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestEncoding(t *testing.T) {
	req := NewRequest(int64(1), "tools/list", map[string]any{})
	data, err := req.Encode()
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if got["jsonrpc"] != "2.0" {
		t.Fatalf("unexpected jsonrpc: %v", got["jsonrpc"])
	}
	if got["method"] != "tools/list" {
		t.Fatalf("unexpected method: %v", got["method"])
	}
	if got["id"] != float64(1) {
		t.Fatalf("unexpected id: %v", got["id"])
	}
	if _, ok := got["params"]; !ok {
		t.Fatalf("params should be present")
	}
}

func TestResponseDecoding(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo"}]}}`)
	resp, err := DecodeResponse(data)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Fatalf("unexpected jsonrpc: %q", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "echo" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestResponseError(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`)
	resp, err := DecodeResponse(data)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(resp.Error.Error(), "-32601") {
		t.Fatalf("unexpected error string: %v", resp.Error)
	}
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("abc", -32000, "boom", map[string]any{"k": "v"})
	data, err := resp.Encode()
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	got, err := DecodeResponse(data)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "abc" || got.Error == nil || got.Error.Code != -32000 || got.Error.Message != "boom" {
		t.Fatalf("unexpected response: %#v", got)
	}
}
