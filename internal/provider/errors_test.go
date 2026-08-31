package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestMapHTTPErrorStatuses(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantKind  Kind
		wantRetry bool
	}{
		{
			name:      "rate limit",
			status:    429,
			body:      "slow down",
			wantKind:  RateLimit,
			wantRetry: true,
		},
		{
			name:      "unauthorized",
			status:    401,
			body:      "",
			wantKind:  AuthError,
			wantRetry: false,
		},
		{
			name:      "forbidden",
			status:    403,
			body:      "",
			wantKind:  AuthError,
			wantRetry: false,
		},
		{
			name:      "payment required",
			status:    402,
			body:      "",
			wantKind:  InsufficientBalance,
			wantRetry: false,
		},
		{
			name:      "server error",
			status:    500,
			body:      "internal",
			wantKind:  ServerError,
			wantRetry: true,
		},
		{
			name:      "service unavailable",
			status:    503,
			body:      "",
			wantKind:  ServerError,
			wantRetry: true,
		},
		{
			name:      "bad request",
			status:    400,
			body:      "",
			wantKind:  BadRequest,
			wantRetry: false,
		},
		{
			name:      "not found",
			status:    404,
			body:      "",
			wantKind:  BadRequest,
			wantRetry: false,
		},
		{
			name:      "unknown",
			status:    200,
			body:      "",
			wantKind:  Unknown,
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapHTTPError(tt.status, tt.body)
			if got == nil {
				t.Fatal("MapHTTPError() returned nil")
			}
			if got.Kind != tt.wantKind {
				t.Fatalf("MapHTTPError(%d, %q).Kind = %q, want %q", tt.status, tt.body, got.Kind, tt.wantKind)
			}
			if got.Retryable != tt.wantRetry {
				t.Fatalf("MapHTTPError(%d, %q).Retryable = %v, want %v", tt.status, tt.body, got.Retryable, tt.wantRetry)
			}
			if got.Message == "" {
				t.Fatalf("MapHTTPError(%d, %q).Message is empty", tt.status, tt.body)
			}
		})
	}
}

func TestMapHTTPErrorBalanceBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "chinese balance", body: "余额不足，请充值"},
		{name: "chinese arrears", body: "账号已欠费"},
		{name: "english balance", body: "insufficient balance"},
		{name: "english quota", body: "quota exceeded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapHTTPError(400, tt.body)
			if got.Kind != InsufficientBalance {
				t.Fatalf("MapHTTPError(400, %q).Kind = %q, want %q", tt.body, got.Kind, InsufficientBalance)
			}
			if got.Retryable {
				t.Fatalf("MapHTTPError(400, %q).Retryable = true, want false", tt.body)
			}
		})
	}
}

func TestMapHTTPErrorRedactsSecrets(t *testing.T) {
	got := MapHTTPError(400, `{"error":"apiKey=sk-1234567890abcdefghijklmnopqrstuvwxyz"}`)
	if got == nil {
		t.Fatal("MapHTTPError() returned nil")
	}
	if strings.Contains(got.Message, "sk-1234567890abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("MapHTTPError leaked API key in message: %q", got.Message)
	}
	if !strings.Contains(got.Message, "***") {
		t.Fatalf("MapHTTPError message does not contain redaction marker: %q", got.Message)
	}
}

func TestIsRetryable(t *testing.T) {
	if IsRetryable(nil) {
		t.Fatal("IsRetryable(nil) = true, want false")
	}
	if IsRetryable(errors.New("plain error")) {
		t.Fatal("IsRetryable(plain error) = true, want false")
	}

	rateLimit := MapHTTPError(429, "")
	if !IsRetryable(rateLimit) {
		t.Fatal("IsRetryable(RateLimit) = false, want true")
	}
	if !IsRetryable(fmt.Errorf("wrapped: %w", rateLimit)) {
		t.Fatal("IsRetryable(wrapped RateLimit) = false, want true")
	}

	auth := MapHTTPError(401, "")
	if IsRetryable(auth) {
		t.Fatal("IsRetryable(AuthError) = true, want false")
	}
	if IsRetryable(fmt.Errorf("wrapped: %w", auth)) {
		t.Fatal("IsRetryable(wrapped AuthError) = true, want false")
	}
}
