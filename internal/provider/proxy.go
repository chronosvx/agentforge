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
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
