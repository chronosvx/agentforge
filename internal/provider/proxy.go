package provider

import (
	"net/http"
	"net/url"
	"time"
)

// newHTTPClient 创建 OpenAI 兼容客户端使用的 HTTP client。
//
// 代理优先级：
//  1. OpenAIConfig.ProxyURL 显式配置的代理；
//  2. 未配置时使用 http.ProxyFromEnvironment，自动读取
//     HTTPS_PROXY / HTTP_PROXY / NO_PROXY 等环境变量。
func newHTTPClient(timeout time.Duration, proxyURL string) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: newHTTPTransport(proxyURL),
	}
}

// newStreamHTTPClient 创建用于 SSE 流式请求的 HTTP client。
// 流式响应可能持续超过 OpenAIConfig.Timeout，因此这里不设置总 Timeout；
// 只保留 ResponseHeaderTimeout，避免服务端一直不返回响应头时无限等待。
func newStreamHTTPClient(proxyURL string) *http.Client {
	transport := newHTTPTransport(proxyURL)
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{
		Transport: transport,
	}
}

func newHTTPTransport(proxyURL string) *http.Transport {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return transport
}
