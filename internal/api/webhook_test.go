package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentApprovalWebhook(t *testing.T) {
	received := make(chan map[string]any, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode webhook body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	notifier := &Notifier{
		Client:  webhookSrv.Client(),
		Webhook: &Webhook{URL: webhookSrv.URL, Secret: "test-secret"},
	}

	srv := NewServer(nil, "", nil)
	srv.OnAgentApproval = func(approval any) {
		notifier.Notify("agent.approval.required", approval)
	}

	srv.FireAgentApproval(map[string]any{
		"sessionId": "s1",
		"actionId":  "action-0",
		"phase":     "AwaitingApproval",
	})

	select {
	case body := <-received:
		if body["event"] != "agent.approval.required" {
			t.Fatalf("unexpected event: %#v", body["event"])
		}
		payload, ok := body["payload"].(map[string]any)
		if !ok {
			t.Fatalf("payload is not an object: %#v", body["payload"])
		}
		if payload["sessionId"] != "s1" || payload["actionId"] != "action-0" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook")
	}
}
