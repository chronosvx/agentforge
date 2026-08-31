// Package mcpserver 提供 MCP Server 基础能力。
//
// 当前实现支持 JSON-RPC 2.0 的 initialize、tools/list、tools/call 三个方法，
// 并提供 stdio 与 HTTP POST 两种传输入口，便于 agentforge 作为 MCP Server
// 对外暴露工具。
package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/chronosvx/agentforge/internal/mcp"
)

const (
	// DefaultProtocolVersion 是 MCP 握手时声明的协议版本。
	DefaultProtocolVersion = "2024-11-05"
	// DefaultServerName 是 MCP serverInfo 中的服务名。
	DefaultServerName = "agentforge"
	// DefaultServerVersion 是 MCP serverInfo 中的服务版本。
	DefaultServerVersion = "0.1.0"
)

// Tool 是 MCP tools/list 返回的工具描述。
type Tool = mcp.Tool

// Handler 是工具执行函数。
type Handler func(args map[string]any) (any, error)

type toolEntry struct {
	tool    Tool
	handler Handler
}

// Server 是 MCP Server 的最小实现。
type Server struct {
	mu    sync.RWMutex
	tools map[string]toolEntry

	serverName      string
	serverVersion   string
	protocolVersion string
}

// New 创建一个 MCP Server。
func New() *Server {
	return &Server{
		tools:           make(map[string]toolEntry),
		serverName:      DefaultServerName,
		serverVersion:   DefaultServerVersion,
		protocolVersion: DefaultProtocolVersion,
	}
}

// RegisterTool 注册一个 MCP 工具。
// 重复注册同名工具会返回错误。
func (s *Server) RegisterTool(name string, handler Handler) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("mcpserver: tool name cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("mcpserver: tool %q handler cannot be nil", name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tools[name]; exists {
		return fmt.Errorf("mcpserver: tool %q already registered", name)
	}
	s.tools[name] = toolEntry{
		tool: Tool{
			Name:        name,
			Description: "MCP tool",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		handler: handler,
	}
	return nil
}

// Handle 处理一条 JSON-RPC 2.0 请求并返回对应的响应字节。
// 对无 ID 的通知消息返回 nil，符合 JSON-RPC 通知语义。
// 协议或工具错误会编码为 JSON-RPC error 响应，不会以 Go error 形式返回。
func (s *Server) Handle(_ context.Context, data []byte) []byte {
	var req mcp.Request
	if err := json.Unmarshal(data, &req); err != nil {
		return encodeResponse(mcp.NewErrorResponse(nil, -32700, "parse error", map[string]any{
			"error": err.Error(),
		}))
	}
	if req.Method == "" {
		return encodeResponse(mcp.NewErrorResponse(req.ID, -32600, "invalid request", nil))
	}
	if req.ID == nil {
		return nil
	}

	switch req.Method {
	case "initialize":
		return encodeResponse(mcp.NewResponse(req.ID, map[string]any{
			"protocolVersion": s.protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    s.serverName,
				"version": s.serverVersion,
			},
		}))
	case "tools/list":
		return encodeResponse(mcp.NewResponse(req.ID, map[string]any{
			"tools": s.listTools(),
		}))
	case "tools/call":
		return s.callTool(req)
	default:
		return encodeResponse(mcp.NewErrorResponse(req.ID, -32601, "method not found", nil))
	}
}

// ServeHTTP 提供 HTTP POST JSON-RPC 传输。
// 每个请求体应包含一条 JSON-RPC 2.0 消息，响应以 application/json 返回。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	resp := s.Handle(r.Context(), body)
	if len(resp) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

// ServeStdio 提供 stdio 传输：从 in 读取换行分隔的 JSON-RPC 请求，
// 将响应写入 out。stdout 只输出 JSON-RPC 消息，符合 MCP stdio 约定。
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	if in == nil {
		return errors.New("mcpserver: stdio input is nil")
	}
	if out == nil {
		return errors.New("mcpserver: stdio output is nil")
	}

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		resp := s.Handle(ctx, line)
		if len(resp) == 0 {
			continue
		}
		if _, err := out.Write(append(resp, '\n')); err != nil {
			return fmt.Errorf("mcpserver: write stdio response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcpserver: read stdio: %w", err)
	}
	return nil
}

func (s *Server) listTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]Tool, 0, len(s.tools))
	for _, entry := range s.tools {
		tools = append(tools, entry.tool)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

func (s *Server) callTool(req mcp.Request) []byte {
	params, _ := req.Params.(map[string]any)
	name, _ := params["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return encodeResponse(mcp.NewErrorResponse(req.ID, -32602, "invalid params: missing tool name", nil))
	}

	s.mu.RLock()
	entry, ok := s.tools[name]
	s.mu.RUnlock()
	if !ok {
		return encodeResponse(mcp.NewErrorResponse(req.ID, -32602, "invalid params: unknown tool", map[string]any{
			"name": name,
		}))
	}

	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	result, err := entry.handler(args)
	if err != nil {
		return encodeResponse(mcp.NewResponse(req.ID, map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": err.Error(),
				},
			},
			"isError": true,
		}))
	}
	return encodeResponse(mcp.NewResponse(req.ID, toolResult(result)))
}

func toolResult(result any) map[string]any {
	if m, ok := result.(map[string]any); ok {
		if _, hasContent := m["content"]; hasContent {
			return m
		}
	}
	text := ""
	if result != nil {
		text = fmt.Sprint(result)
	}
	return map[string]any{
		"content": []any{
			map[string]any{
				"type": "text",
				"text": text,
			},
		},
		"isError": false,
	}
}

func encodeResponse(resp mcp.Response) []byte {
	data, err := resp.Encode()
	if err != nil {
		// Response 由固定结构组成，正常情况下不会编码失败。
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return data
}
