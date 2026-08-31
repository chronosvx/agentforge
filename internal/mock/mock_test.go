package mock_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/mock"
	"github.com/coder/websocket"
)

func TestMockOpenAndInvoke(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := mock.Start(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start mock server: %v", err)
	}
	defer srv.Close()
	if srv.Addr() == "" {
		t.Fatal("mock server Addr is empty")
	}

	conn, _, err := websocket.Dial(ctx, "ws://"+srv.Addr()+"/agentforge/v1/connection", nil)
	if err != nil {
		t.Fatalf("dial mock server: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	// open
	mustWrite(t, conn, map[string]any{
		"operation": "open",
		"value": map[string]any{
			"workspace": "ws",
			"sessionId": "sess",
			"options":   map[string]any{},
		},
	})
	openResp := mustRead(t, conn)
	if openResp["operation"] != "open" {
		t.Fatalf("expected open response, got %v", openResp)
	}
	openValue, _ := openResp["value"].(map[string]any)
	channelID, _ := openValue["channelId"].(string)
	if channelID == "" {
		t.Fatal("open response missing channelId")
	}

	// subscribe
	mustWrite(t, conn, map[string]any{
		"operation": "subscribe",
		"value": map[string]any{
			"channelId":    channelID,
			"afterVersion": 0,
		},
	})
	subResp := mustRead(t, conn)
	if subResp["operation"] != "subscribe" {
		t.Fatalf("expected subscribe response, got %v", subResp)
	}

	// invoke chat
	mustWrite(t, conn, map[string]any{
		"operation": "invoke",
		"value": map[string]any{
			"channelId": channelID,
			"method":    "chat",
			"arguments": []any{"hello"},
		},
	})

	// 读取事件直到 invoke 最终响应，并确认流式回复出现。
	sawEvent := false
	for {
		resp := mustRead(t, conn)
		op, _ := resp["operation"].(string)
		if op == "invoke" {
			break
		}
		if op == "event" {
			sawEvent = true
		}
	}
	if !sawEvent {
		t.Fatal("expected at least one event before invoke response")
	}
}

func mustWrite(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func mustRead(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return v
}
