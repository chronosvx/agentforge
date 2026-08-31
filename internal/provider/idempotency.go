package provider

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// 请求幂等相关 HTTP header 名称。
const (
	// RequestIDHeader 是 OpenAI 兼容客户端用于追踪请求的 header。
	RequestIDHeader = "X-Request-Id"
	// IdempotencyKeyHeader 是部分厂商用于幂等写入的 header。
	IdempotencyKeyHeader = "Idempotency-Key"
)

// RequestID 生成一个随机 UUID v4 风格的请求 ID。
// 使用 crypto/rand 保证每次调用得到不同的值，可用于 X-Request-Id / Idempotency-Key。
func RequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 在常见平台不会失败；这里兜底返回一个基于时间的可辨识 ID，
		// 避免因随机源异常导致请求完全无法发起。
		return fallbackRequestID()
	}
	// 设置为 UUID v4。
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// WithRequestID 把请求 ID 写入 ChatRequest 的 Headers。
// 返回新的 ChatRequest；原请求不会被修改。
func WithRequestID(req ChatRequest, id string) ChatRequest {
	if id == "" {
		return req
	}
	headers := make(map[string]string, len(req.Headers)+1)
	for k, v := range req.Headers {
		headers[k] = v
	}
	headers[RequestIDHeader] = id
	req.Headers = headers
	return req
}

// fallbackRequestID 是 crypto/rand 不可用时的极简兜底，仍尽量保持唯一。
func fallbackRequestID() string {
	return "req-" + strconv.FormatInt(time.Now().UnixNano(), 16)
}
