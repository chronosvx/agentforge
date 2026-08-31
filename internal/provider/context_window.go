// Package provider 提供直连厂商层的公共类型与默认配置。
//
// 当前先落地厂商/模型成本单价表与模型上下文窗口注册表，后续可在本包继续扩展厂商直连客户端、
// 鉴权与模型路由等能力。
package provider

import "strings"

// ModelInfo 描述一个模型的上下文窗口与能力信息。
//
// ContextWindow 表示模型支持的上下文总长度（tokens）；MaxOutput 表示单次最大输出 tokens；
// SupportsTools / SupportsStreaming 分别表示是否支持工具调用与流式输出。
type ModelInfo struct {
	Name              string
	ContextWindow     int64
	MaxOutput         int64
	SupportsTools     bool
	SupportsStreaming bool
}

// ModelRegistry 是模型上下文窗口信息的注册与查询入口。
//
// 未注册的模型会返回默认模型（DeepSeek-chat）作为兜底，同时 Get 的 bool 返回 false
// 表示“未精确命中，使用了兜底信息”。
type ModelRegistry struct {
	models map[string]ModelInfo
}

// NewModelRegistry 返回空的模型注册表。
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{models: make(map[string]ModelInfo)}
}

// DefaultModelInfo 返回默认兜底模型信息（DeepSeek-chat）。
//
// 该信息作为未匹配模型名时的兜底，与 DefaultModelRegistry 的预置默认模型保持一致。
func DefaultModelInfo() ModelInfo {
	return ModelInfo{
		Name:              "DeepSeek-chat",
		ContextWindow:     64 * 1024,
		MaxOutput:         8 * 1024,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}

// DefaultModelRegistry 返回预置常用模型上下文窗口信息的注册表。
func DefaultModelRegistry() *ModelRegistry {
	r := NewModelRegistry()
	for _, info := range defaultModelInfos() {
		r.Register(info)
	}
	return r
}

// Register 注册或覆盖一个模型的信息。模型名不区分大小写，且会忽略首尾空白。
func (r *ModelRegistry) Register(info ModelInfo) {
	if r == nil {
		return
	}
	if r.models == nil {
		r.models = make(map[string]ModelInfo)
	}
	key := normalizeModelName(info.Name)
	if key == "" {
		return
	}
	r.models[key] = info
}

// Get 查询指定模型的信息。
//
// 精确命中时返回对应 ModelInfo 且 ok=true；未命中时返回默认兜底模型信息且 ok=false，
// 方便调用方判断是否需要提示“使用默认上下文窗口”。
func (r *ModelRegistry) Get(name string) (ModelInfo, bool) {
	if r != nil {
		if info, ok := r.models[normalizeModelName(name)]; ok {
			return info, true
		}
	}
	return DefaultModelInfo(), false
}

// Default 返回默认兜底模型信息。
func (r *ModelRegistry) Default() ModelInfo {
	return DefaultModelInfo()
}

func normalizeModelName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func defaultModelInfos() []ModelInfo {
	return []ModelInfo{
		// DeepSeek
		{
			Name: "DeepSeek-chat", ContextWindow: 64 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "deepseek-reasoner", ContextWindow: 64 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		// OpenAI
		{
			Name: "gpt-4o", ContextWindow: 128 * 1024, MaxOutput: 16 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "gpt-4o-mini", ContextWindow: 128 * 1024, MaxOutput: 16 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		// Moonshot / Kimi
		{
			Name: "kimi-k2", ContextWindow: 128 * 1024, MaxOutput: 32 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		// Zhipu
		{
			Name: "glm-4-plus", ContextWindow: 128 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "glm-4-flash", ContextWindow: 128 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		// Qwen
		{
			Name: "qwen-max", ContextWindow: 32 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "qwen-plus", ContextWindow: 128 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		// Ollama 本地模型
		{
			Name: "qwen2.5:7b", ContextWindow: 32 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "qwen2.5:14b", ContextWindow: 32 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "llama3.1:8b", ContextWindow: 128 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "llama3.2:3b", ContextWindow: 128 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
		{
			Name: "deepseek-r1:7b", ContextWindow: 64 * 1024, MaxOutput: 8 * 1024,
			SupportsTools: true, SupportsStreaming: true,
		},
	}
}
