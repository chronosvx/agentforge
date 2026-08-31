// Package mcp 提供 Model Context Protocol (MCP) 客户端基础能力。
//
// 当前实现聚焦于 JSON-RPC 2.0 消息封装、HTTP/SSE 与 stdio 两种传输，
// 支持 initialize、tools/list、tools/call 三个基础方法。
package mcp

import (
	"encoding/json"
	"fmt"
)

// Request 是 JSON-RPC 2.0 请求消息。
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// Response 是 JSON-RPC 2.0 响应消息。
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError 是 JSON-RPC 2.0 错误对象，同时实现 error 接口。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// NewRequest 构造一个 JSON-RPC 2.0 请求。
func NewRequest(id any, method string, params any) Request {
	return Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

// Encode 将请求编码为 JSON 字节。
func (r Request) Encode() ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}
	return data, nil
}

// NewResponse 构造一个 JSON-RPC 2.0 成功响应。
func NewResponse(id any, result any) Response {
	var raw json.RawMessage
	if result != nil {
		raw, _ = json.Marshal(result)
	}
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  raw,
	}
}

// NewErrorResponse 构造一个 JSON-RPC 2.0 错误响应。
func NewErrorResponse(id any, code int, message string, data any) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// Encode 将响应编码为 JSON 字节。
func (r Response) Encode() ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal response: %w", err)
	}
	return data, nil
}

// DecodeResponse 解析 JSON-RPC 2.0 响应消息。
func DecodeResponse(data []byte) (*Response, error) {
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("mcp: decode response: %w", err)
	}
	if r.JSONRPC != "" && r.JSONRPC != "2.0" {
		return nil, fmt.Errorf("mcp: unsupported jsonrpc version %q", r.JSONRPC)
	}
	return &r, nil
}

// UnmarshalResult 将响应中的 result 解码到 v。
func (r *Response) UnmarshalResult(v any) error {
	if r == nil {
		return fmt.Errorf("mcp: nil response")
	}
	if len(r.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(r.Result, v); err != nil {
		return fmt.Errorf("mcp: decode result: %w", err)
	}
	return nil
}

// Error 实现 error 接口。
func (e *RPCError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("mcp: jsonrpc error %d: %s", e.Code, e.Message)
}
