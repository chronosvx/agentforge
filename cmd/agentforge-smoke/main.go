// Command agentforge-smoke 用于快速验证与真实 DSH Web API 的连接。
//
// 用法：
//
//	go run ./cmd/agentforge-smoke --url http://127.0.0.1:54695 --prompt "你好"
//	go run ./cmd/agentforge-smoke --url http://127.0.0.1:54695 --resume <sessionID> --prompt "继续"
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/dshapi"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:17891", "DSH Web API base URL")
	prompt := flag.String("prompt", "", "发送一条 prompt 并观察 mux 事件")
	resume := flag.String("resume", "", "复用已有会话 ID，不创建新会话")
	duration := flag.Duration("watch", 5*time.Second, "观察 mux 事件的时长")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *duration+15*time.Second)
	defer cancel()

	cl := dshapi.New(*url)

	var describe map[string]any
	if err := cl.Call(ctx, "host.describe", map[string]any{}, &describe); err != nil {
		fatalf("host.describe: %v", err)
	}
	fmt.Printf("host.describe: %s\n", jsonSummary(describe))

	sessions, err := cl.ListSessions(ctx)
	if err != nil {
		fatalf("session.list: %v", err)
	}
	fmt.Printf("session.list: %d sessions\n", len(sessions.Items))

	sessionID := *resume
	if sessionID == "" {
		created, err := cl.CreateSession(ctx, dshapi.CreateSessionRequest{Cwd: mustCwd()})
		if err != nil {
			fatalf("session.create: %v", err)
		}
		sessionID = created.SessionID
		fmt.Printf("session.create: %s\n", sessionID)
	} else {
		found := false
		for _, s := range sessions.Items {
			if s.SessionID == sessionID {
				found = true
				break
			}
		}
		if found {
			fmt.Printf("resume: found session %s in session.list\n", sessionID)
		} else {
			fmt.Printf("resume: using session %s (not in session.list, will try directly)\n", sessionID)
		}
	}
	fmt.Printf("session.id=%s\n", sessionID)

	if *prompt == "" {
		return
	}

	mux, err := cl.OpenMux(ctx)
	if err != nil {
		fatalf("events.mux: %v", err)
	}

	if _, err := cl.Prompt(ctx, sessionID, *prompt, "queue"); err != nil {
		fatalf("session.prompt: %v", err)
	}
	fmt.Println("prompt accepted, watching mux events...")

	watchCtx, cancelWatch := context.WithTimeout(ctx, *duration)
	defer cancelWatch()
	deadline := time.After(*duration)

	projections := map[string]bool{}
	lastMuxSeq := int64(0)
	events := 0

watchLoop:
	for {
		select {
		case frame, ok := <-mux:
			if !ok {
				fmt.Println("mux closed")
				break watchLoop
			}
			events++
			fmt.Printf("mux: %s\n", jsonSummary(frame))
			if key, ok := projectionKey(frame); ok {
				projections[key] = true
			}
			if seq, ok := muxSeq(frame); ok && seq > lastMuxSeq {
				lastMuxSeq = seq
			}
		case <-deadline:
			fmt.Println("watch timeout")
			break watchLoop
		case <-watchCtx.Done():
			fmt.Println("watch done")
			break watchLoop
		}
	}

	fmt.Printf("watch.summary: events=%d lastSeq=%d\n", events, lastMuxSeq)
	printProjections(projections)
	printHistory(ctx, cl, sessionID)
}

func projectionKey(frame dshapi.ServerRequest) (string, bool) {
	var payload map[string]any
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return "", false
	}
	typ, _ := payload["type"].(string)
	if typ != "session/projection" {
		return "", false
	}
	key, _ := payload["key"].(string)
	return key, key != ""
}

func muxSeq(frame dshapi.ServerRequest) (int64, bool) {
	var payload map[string]any
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return 0, false
	}
	typ, _ := payload["type"].(string)
	switch typ {
	case "session/event":
		event, ok := payload["event"].(map[string]any)
		if !ok {
			return 0, false
		}
		seq, _ := event["seq"].(float64)
		return int64(seq), seq > 0
	case "session/projection":
		seq, _ := payload["seq"].(float64)
		return int64(seq), seq > 0
	default:
		return 0, false
	}
}

func printProjections(seen map[string]bool) {
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range []string{"sessionStats", "contextBreakdown", "cost", "tokenUsage"} {
		fmt.Printf("projection.%s=%v\n", name, seen[name])
	}
	if len(keys) == 0 {
		fmt.Println("projection.seen=<none>")
	} else {
		fmt.Printf("projection.seen=%s\n", strings.Join(keys, ","))
	}
}

func printHistory(ctx context.Context, cl *dshapi.Client, sessionID string) {
	hist, err := cl.History(ctx, sessionID, nil, nil)
	if err != nil {
		fatalf("session.history: %v", err)
	}

	messages := 0
	lastSeq := int64(0)
	hasUser := false
	roles := make([]string, 0, len(hist.Events))
	for _, entry := range hist.Events {
		if entry.Event.Seq > lastSeq {
			lastSeq = entry.Event.Seq
		}
		switch entry.Event.Type {
		case "user/message", "assistant/message":
			messages++
			roles = append(roles, fmt.Sprintf("%s:%d", entry.Event.Type, entry.Event.Seq))
			if entry.Event.Type == "user/message" {
				hasUser = true
			}
		}
	}

	fmt.Printf("history: messages=%d lastSeq=%d hasUser=%v\n", messages, lastSeq, hasUser)
	if len(roles) > 0 {
		fmt.Printf("history.messages: %s\n", strings.Join(roles, ", "))
	}
}

func mustCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func jsonSummary(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	if len(data) > 500 {
		return string(data[:500]) + "..."
	}
	return string(data)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "agentforge-smoke: "+format+"\n", args...)
	os.Exit(1)
}
