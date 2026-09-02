package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/i18n"
)

// updateTPS 根据最新的 output token 数更新每秒生成速度。
// token 数可能来自 tokenUsage 或 context，且可能是 float64 或 int。
func (m *Model) updateTPS() {
	current, ok := currentOutputTokens(m.tokenUsage, m.context)
	if !ok {
		return
	}

	now := time.Now()
	if current > float64(m.lastTokenTotal) {
		elapsed := now.Sub(m.lastTokenAt)
		// 第一次采样时 lastTokenAt 为零值，只建立基线，不计算 tps。
		if !m.lastTokenAt.IsZero() && elapsed > 0 {
			m.tps = (current - float64(m.lastTokenTotal)) / elapsed.Seconds()
		}
		m.lastTokenTotal = int(current)
	}
	m.lastTokenAt = now
}

// currentOutputTokens 从 tokenUsage 或 context 中读取 outputTokens。
func currentOutputTokens(tokenUsage, context map[string]any) (float64, bool) {
	if v, ok := tokenUsage["outputTokens"]; ok {
		if n, ok := numberToFloat(v); ok {
			return n, true
		}
	}
	if v, ok := context["outputTokens"]; ok {
		if n, ok := numberToFloat(v); ok {
			return n, true
		}
	}
	return 0, false
}

// numberToFloat 兼容 float64/int 等常见数值类型。
func numberToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// hudLabel 优先使用 i18n 文案；语言包尚未提供时回退到内置英文短标签。
func hudLabel(key, fallback string) string {
	label := i18n.T(key)
	if label == key {
		return fallback
	}
	return label
}

// renderHUD 渲染紧凑的 token/cost/tps 状态信息。
func (m *Model) renderHUD() string {
	var parts []string

	if tokens, ok := currentOutputTokens(m.tokenUsage, m.context); ok {
		parts = append(parts, fmt.Sprintf("%s: %s", hudLabel("hud.tokens", "tokens"), strconv.FormatFloat(tokens, 'f', -1, 64)))
	}
	if m.cost != nil {
		if cost, ok := m.cost["estimatedYuan"]; ok && cost != nil {
			costStr := fmt.Sprint(cost)
			if costStr != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", hudLabel("hud.cost", "cost"), costStr))
			}
		}
	}
	if m.tps > 0 {
		parts = append(parts, fmt.Sprintf("%s: %s", hudLabel("hud.tps", "tps"), strconv.FormatFloat(m.tps, 'f', -1, 64)))
	}

	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}
