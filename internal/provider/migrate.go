package provider

import (
	"errors"
	"fmt"
	"strings"
)

// providerMessage 是厂商迁移使用的通用消息结构。
// 当前直接复用 provider.Message，保持与 Provider 层已有类型一致。
type providerMessage = Message

// ErrUnsupportedProvider 表示会话迁移目标厂商暂不支持。
var ErrUnsupportedProvider = errors.New("provider: unsupported provider for session conversion")

// openAICompatibleProviders 是当前已知支持 OpenAI 兼容消息格式的厂商。
var openAICompatibleProviders = map[string]struct{}{
	"openai":   {},
	"deepseek": {},
	"moonshot": {},
	"kimi":     {},
	"zhipu":    {},
	"qwen":     {},
	"ollama":   {},
}

// ToOpenAIMessages 把通用消息列表转换为 OpenAI 兼容的 messages 列表。
//
// 转换规则：
//   - system/user：保留 role 与 content；
//   - assistant：保留 role/content，并把 ToolCalls 展开为 OpenAI 的
//     tool_calls 数组（id/type/function.name/function.arguments）；
//   - tool：保留 role/content，并写入 tool_call_id 用于关联 assistant 的工具调用。
func ToOpenAIMessages(msgs []providerMessage) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		msg := map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}

		switch strings.ToLower(strings.TrimSpace(m.Role)) {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				calls := make([]map[string]any, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					typ := tc.Type
					if typ == "" {
						typ = "function"
					}
					calls = append(calls, map[string]any{
						"id":   tc.ID,
						"type": typ,
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": tc.Arguments,
						},
					})
				}
				msg["tool_calls"] = calls
			}
		case "tool":
			if m.ToolCallID != "" {
				msg["tool_call_id"] = m.ToolCallID
			}
		}

		out = append(out, msg)
	}
	return out
}

// ConvertSessionForProvider 把一组 OpenAI 兼容 messages 转换为指定厂商的会话消息格式。
//
// 当前 deepseek、moonshot/kimi、zhipu、qwen、ollama 等厂商均使用 OpenAI
// 兼容协议，因此直接返回原消息；对于暂不支持的厂商返回 ErrUnsupportedProvider，
// 避免上层误把不兼容格式发送给目标厂商。
func ConvertSessionForProvider(provider string, messages []map[string]any) ([]map[string]any, error) {
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" {
		return append([]map[string]any(nil), messages...), nil
	}
	if _, ok := openAICompatibleProviders[name]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, provider)
	}
	return append([]map[string]any(nil), messages...), nil
}
