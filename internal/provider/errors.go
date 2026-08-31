package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/chronosvx/agentforge/internal/security"
)

// Kind 表示厂商错误的分类。
type Kind string

// 支持的错误分类。
const (
	RateLimit           Kind = "rate_limit"
	InsufficientBalance Kind = "insufficient_balance"
	AuthError           Kind = "auth_error"
	Timeout             Kind = "timeout"
	Network             Kind = "network"
	BadRequest          Kind = "bad_request"
	ServerError         Kind = "server_error"
	Unknown             Kind = "unknown"
)

// ProviderError 是直连厂商层使用的统一错误类型。
type ProviderError struct {
	Kind      Kind
	Message   string
	Retryable bool
}

// Error 实现 error 接口。
func (e *ProviderError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("provider: %s: %s", e.Kind, e.Message)
}

// MapHTTPError 将 HTTP 状态码与响应体映射为 ProviderError。
//
// 映射规则：
//   - 429  -> RateLimit
//   - 401/403 -> AuthError
//   - 402 或响应体包含余额/欠费关键词 -> InsufficientBalance
//   - 5xx  -> ServerError
//   - 其他 4xx -> BadRequest
//   - 其他状态码 -> Unknown
func MapHTTPError(statusCode int, body string) *ProviderError {
	detail := security.Redact(strings.TrimSpace(body))
	message := func(base string) string {
		if detail == "" {
			return base
		}
		return base + "：" + detail
	}

	switch {
	case statusCode == 429:
		return &ProviderError{
			Kind:      RateLimit,
			Message:   message("请求过于频繁，已被限流，请稍后重试"),
			Retryable: true,
		}
	case statusCode == 401 || statusCode == 403:
		return &ProviderError{
			Kind:      AuthError,
			Message:   message("API Key 无效或权限不足"),
			Retryable: false,
		}
	case statusCode == 402 || isBalanceError(detail):
		return &ProviderError{
			Kind:      InsufficientBalance,
			Message:   message("账户余额不足或已欠费，请充值后重试"),
			Retryable: false,
		}
	case statusCode >= 500:
		return &ProviderError{
			Kind:      ServerError,
			Message:   message("服务端暂时不可用，请稍后重试"),
			Retryable: true,
		}
	case statusCode >= 400:
		return &ProviderError{
			Kind:      BadRequest,
			Message:   message("请求参数错误或不被支持"),
			Retryable: false,
		}
	default:
		return &ProviderError{
			Kind:      Unknown,
			Message:   message("未知错误"),
			Retryable: false,
		}
	}
}

// IsRetryable 判断错误是否可重试。支持直接传入 *ProviderError，
// 也支持通过 %w 包装后的错误。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pe *ProviderError
	if errors.As(err, &pe) && pe != nil {
		return pe.Retryable
	}
	return false
}

func isBalanceError(body string) bool {
	lower := strings.ToLower(body)
	keywords := []string{
		"insufficient balance",
		"insufficient_balance",
		"balance insufficient",
		"not enough balance",
		"arrears",
		"quota exceeded",
		"out of quota",
		"no quota",
		"欠费",
		"余额不足",
		"余额",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
