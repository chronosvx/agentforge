package security

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	bearerTokenRE = regexp.MustCompile(`(?i)\b(Bearer\s+)([A-Za-z0-9._~+/\-=]+)`)
	keyValueRE    = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|authorization|client[_-]?secret|secret|token|password|private[_-]?key|key)\b\s*[:=]\s*("([^"]*)"|'([^']*)'|([^\s,;]+))`)
	knownKeyRE    = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9]{8,}|pk-[A-Za-z0-9]{8,}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16,}|ASIA[0-9A-Z]{16,}|ya29\.[A-Za-z0-9_-]{10,}|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})`)
	longTokenRE   = regexp.MustCompile(`[A-Za-z0-9._~+/\-=]{16,}`)
)

// Redact 将疑似 API Key、Bearer Token 或长密钥替换为 ***。
//
// 目前处理：
//   - "Bearer <token>"
//   - 常见键值对：apiKey、token、authorization、secret、password、key 等
//   - 常见厂商密钥前缀：sk-、pk-、ghp_、xoxb-、AKIA、JWT 等
//   - 长度 >= 20 的连续 token，以及 16-19 字符的高熵片段
func Redact(s string) string {
	if s == "" {
		return s
	}

	s = bearerTokenRE.ReplaceAllString(s, `${1}***`)
	s = keyValueRE.ReplaceAllString(s, `${1}=***`)
	s = knownKeyRE.ReplaceAllString(s, `***`)

	return longTokenRE.ReplaceAllStringFunc(s, func(token string) string {
		if looksLikeSecret(token) {
			return "***"
		}
		return token
	})
}

// RedactMap 递归脱敏 map 中的常见敏感字段。
//
// 敏感字段名（大小写不敏感，忽略 -/_）包括：apiKey、token、authorization、
// key、secret，以及常见的 accessToken、authToken、clientSecret、privateKey、
// password、apiSecret。map 会被原地修改。
func RedactMap(m map[string]any) {
	redactMap(m)
}

func redactMap(m map[string]any) {
	for k, v := range m {
		if isSensitiveKey(k) {
			switch val := v.(type) {
			case map[string]any:
				redactMap(val)
			case []any:
				redactSlice(val)
			default:
				m[k] = "***"
			}
			continue
		}

		switch val := v.(type) {
		case map[string]any:
			redactMap(val)
		case []any:
			redactSlice(val)
		}
	}
}

func redactSlice(s []any) {
	for i, v := range s {
		switch val := v.(type) {
		case map[string]any:
			redactMap(val)
		case []any:
			redactSlice(val)
		case string:
			if looksLikeSecret(val) {
				s[i] = "***"
			}
		}
	}
}

func isSensitiveKey(k string) bool {
	norm := strings.ToLower(k)
	norm = strings.NewReplacer("-", "", "_", "", " ", "").Replace(norm)
	switch norm {
	case "apikey", "token", "authorization", "key", "secret",
		"accesstoken", "authtoken", "clientsecret", "privatekey",
		"password", "apisecret":
		return true
	}
	return false
}

func looksLikeSecret(s string) bool {
	if len(s) < 16 {
		return false
	}
	// 20 字符以上的连续 token 默认视为疑似长密钥；16-19 字符要求具备
	// 多种字符类型（大小写/数字/符号），降低误伤普通文本的概率。
	if len(s) >= 20 {
		return true
	}

	hasLower, hasUpper, hasDigit, hasSymbol := false, false, false, false
	for _, r := range s {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case strings.ContainsRune("._~+/-=", r):
			hasSymbol = true
		}
	}

	classes := 0
	for _, ok := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if ok {
			classes++
		}
	}
	return classes >= 2
}
