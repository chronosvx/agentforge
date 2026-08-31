package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// benchSSEStream 生成 n 个文本 delta 的 SSE 数据，模拟流式响应。
func benchSSEStream(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"chunk-%d \"}}]}\n\n", i)
	}
	sb.WriteString("data: [DONE]\n\n")
	return sb.String()
}

func BenchmarkOpenAISSEParse1000Chunks(b *testing.B) {
	data := benchSSEStream(1000)
	c := &openAIClient{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := io.NopCloser(strings.NewReader(data))
		ch := make(chan StreamEvent, 2048)
		c.readStream(context.Background(), &http.Response{Body: body}, ch)
	}
}
