package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
)

// Tool 是 MCP tools/list 返回的工具描述。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// Client 是 MCP 客户端基础实现。
//
// 传输方式由 New 的 url 参数自动判断：
//   - http:// 或 https:// 开头：HTTP/SSE 传输，按 JSON-RPC over HTTP POST 调用；
//   - 其他字符串：stdio 传输，按可执行命令启动子进程，通过 stdin/stdout 交换 JSON-RPC。
type Client struct {
	url string

	httpClient *http.Client
	sessionID  string

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *bytes.Buffer
	started bool

	mu     sync.Mutex
	nextID int64
	closed bool
}

// New 创建一个 MCP 客户端。
func New(url string) *Client {
	return &Client{
		url:        strings.TrimSpace(url),
		httpClient: &http.Client{},
	}
}

// Initialize 发送 MCP initialize 请求，完成握手。
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "agentforge",
			"version": "0.1.0",
		},
	}
	return c.call(ctx, "initialize", params, nil)
}

// ListTools 发送 tools/list 请求，返回工具列表。
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := c.call(ctx, "tools/list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	if out.Tools == nil {
		return []Tool{}, nil
	}
	return out.Tools, nil
}

// CallTool 发送 tools/call 请求，返回 MCP 服务端原始结果。
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	var result any
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Close 关闭 stdio 子进程（HTTP 客户端无需额外关闭）。
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Wait()
	}
	return nil
}

// call 生成 ID 并分发到对应传输。
func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	req := NewRequest(id, method, params)
	if c.isHTTP() {
		return c.callHTTP(ctx, req, result)
	}
	return c.callStdio(ctx, req, result)
}

func (c *Client) isHTTP() bool {
	return strings.HasPrefix(c.url, "http://") || strings.HasPrefix(c.url, "https://")
}

func (c *Client) callHTTP(ctx context.Context, req Request, result any) error {
	body, err := req.Encode()
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mcp: new http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	c.mu.Lock()
	sessionID := c.sessionID
	c.mu.Unlock()
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: post %s: %w", c.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("mcp: http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		c.mu.Lock()
		c.sessionID = id
		c.mu.Unlock()
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("mcp: read http response: %w", err)
	}

	// 兼容返回 text/event-stream 的 MCP 服务端。
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		data, err = parseSSEData(data)
		if err != nil {
			return err
		}
	}

	rpcResp, err := DecodeResponse(data)
	if err != nil {
		return err
	}
	if !sameID(rpcResp.ID, req.ID) {
		return fmt.Errorf("mcp: response id mismatch: sent %v got %v", req.ID, rpcResp.ID)
	}
	if rpcResp.Error != nil {
		return rpcResp.Error
	}
	if result != nil {
		if err := rpcResp.UnmarshalResult(result); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) callStdio(ctx context.Context, req Request, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("mcp: client closed")
	}
	if err := c.ensureStdioLocked(); err != nil {
		return err
	}

	body, err := req.Encode()
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if _, err := c.stdin.Write(body); err != nil {
		return fmt.Errorf("mcp: write stdio: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return fmt.Errorf("mcp: read stdio: %w", err)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		rpcResp, err := DecodeResponse(line)
		if err != nil {
			// stdio 的 stdout 应按 MCP 约定只输出 JSON-RPC 消息。
			return fmt.Errorf("mcp: decode stdio response: %w", err)
		}
		if !sameID(rpcResp.ID, req.ID) {
			// 忽略服务端主动发送的通知或其它请求的响应。
			continue
		}
		if rpcResp.Error != nil {
			return rpcResp.Error
		}
		if result != nil {
			if err := rpcResp.UnmarshalResult(result); err != nil {
				return err
			}
		}
		return nil
	}
}

// ensureStdioLocked 在已持有 c.mu 的前提下启动 stdio 子进程。
func (c *Client) ensureStdioLocked() error {
	if c.started {
		return nil
	}
	parts := splitCommand(c.url)
	if len(parts) == 0 {
		return fmt.Errorf("mcp: empty stdio command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp: stdio stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp: stdio stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcp: start stdio %q: %w", c.url, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewReader(stdout)
	c.stderr = &stderr
	c.started = true
	return nil
}

func splitCommand(s string) []string {
	// 简单按空白切分，支持 `go run file.go`、`python3 server.py` 等形式。
	return strings.Fields(s)
}

func parseSSEData(data []byte) ([]byte, error) {
	var payload []byte
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, "data:") {
			if line == "" && len(payload) > 0 {
				trimmed := bytes.TrimSpace(payload)
				if len(trimmed) > 0 && trimmed[0] == '{' {
					return trimmed, nil
				}
				payload = nil
			}
			continue
		}
		value := strings.TrimPrefix(line, "data:")
		value = strings.TrimPrefix(value, " ")
		if len(payload) > 0 {
			payload = append(payload, '\n')
		}
		payload = append(payload, value...)
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("mcp: empty SSE response")
	}
	return trimmed, nil
}

func sameID(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}
