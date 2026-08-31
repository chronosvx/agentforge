package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Webhook 描述一个事件通知目标。
type Webhook struct {
	URL    string
	Secret string
}

// Notifier 异步发送 Webhook 事件。
type Notifier struct {
	Client  *http.Client
	Webhook *Webhook
}

// Notify 异步 POST JSON 事件到 Webhook URL。
func (n *Notifier) Notify(event string, payload any) {
	if n == nil || n.Webhook == nil || n.Webhook.URL == "" {
		return
	}
	body, err := json.Marshal(map[string]any{"event": event, "payload": payload, "time": time.Now()})
	if err != nil {
		return
	}
	go func() {
		client := n.Client
		if client == nil {
			client = http.DefaultClient
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.Webhook.URL, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if n.Webhook.Secret != "" {
			req.Header.Set("X-Webhook-Secret", n.Webhook.Secret)
		}
		_, _ = client.Do(req)
	}()
}
