// Package provider 提供直连厂商层的公共类型与默认配置。
//
// 当前先落地厂商/模型成本单价表，后续可在本包继续扩展厂商直连客户端、
// 鉴权与模型路由等能力。
package provider

import (
	"strings"

	"github.com/chronosvx/agentforge/internal/cost"
)

// PlanType 表示计费模式。
// 类型定义在 internal/cost，这里通过别名暴露为 provider.PlanType。
type PlanType = cost.PlanType

const (
	// PlanTypePayAsYouGo 表示按量付费（按 token 用量计费）。
	PlanTypePayAsYouGo = cost.PlanTypePayAsYouGo
	// PlanTypeTokenPlan 表示 Token Plan / 包月套餐（按固定月费计费）。
	PlanTypeTokenPlan = cost.PlanTypeTokenPlan
)

// Pricing 描述一个厂商/模型的动态成本单价。
// 类型定义在 internal/cost，这里通过别名暴露为 provider.Pricing。
type Pricing = cost.Pricing

// PricingTable 是厂商/模型成本单价表的注册与查询入口。
//
// 未注册的 provider/model 会返回 DeepSeek 默认价格作为兜底，同时
// Get 的 bool 返回 false 表示“未精确命中，使用了兜底价格”。
type PricingTable struct {
	prices map[string]Pricing
}

// NewPricingTable 返回空的定价表。
func NewPricingTable() *PricingTable {
	return &PricingTable{prices: make(map[string]Pricing)}
}

// DefaultPricing 返回 DeepSeek-chat 的默认估算价格（元 / 百万 tokens）。
//
// 该价格与 internal/cost 的默认常量保持一致，作为未匹配厂商/模型时的兜底。
func DefaultPricing() Pricing {
	return Pricing{
		Provider:             "DeepSeek",
		Model:                "DeepSeek-chat",
		InputPerMillion:      1.5,
		OutputPerMillion:     4.5,
		CacheReadPerMillion:  0.05,
		CacheWritePerMillion: 1.5,
		Currency:             "CNY",
		PlanType:             PlanTypePayAsYouGo,
	}
}

// DefaultPricingTable 返回预置常用厂商/模型定价的定价表。
func DefaultPricingTable() *PricingTable {
	t := NewPricingTable()
	for _, p := range defaultPricings() {
		t.Register(p)
	}
	return t
}

// Register 注册或覆盖一个厂商/模型的定价。
func (t *PricingTable) Register(p Pricing) {
	if t == nil {
		return
	}
	if t.prices == nil {
		t.prices = make(map[string]Pricing)
	}
	t.prices[tableKey(p.Provider, p.Model)] = p
}

// Get 查询指定厂商/模型的定价。
//
// 精确命中时返回对应 Pricing 且 ok=true；未命中时返回 DeepSeek 默认兜底价格
// 且 ok=false，方便调用方判断是否需要提示“使用默认价格”。
func (t *PricingTable) Get(provider, model string) (Pricing, bool) {
	if t != nil {
		if p, ok := t.prices[tableKey(provider, model)]; ok {
			return p, true
		}
	}
	return DefaultPricing(), false
}

// All 返回当前定价表中的全部定价，顺序不保证。
func (t *PricingTable) All() []Pricing {
	if t == nil || t.prices == nil {
		return nil
	}
	out := make([]Pricing, 0, len(t.prices))
	for _, p := range t.prices {
		out = append(out, p)
	}
	return out
}

func tableKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "\x00" + strings.ToLower(strings.TrimSpace(model))
}

func defaultPricings() []Pricing {
	return []Pricing{
		// DeepSeek
		{
			Provider: "DeepSeek", Model: "DeepSeek-chat",
			InputPerMillion: 1.5, OutputPerMillion: 4.5,
			CacheReadPerMillion: 0.05, CacheWritePerMillion: 1.5,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		{
			Provider: "DeepSeek", Model: "deepseek-reasoner",
			InputPerMillion: 4, OutputPerMillion: 16,
			CacheReadPerMillion: 1, CacheWritePerMillion: 4,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		// OpenAI
		{
			Provider: "OpenAI", Model: "gpt-4o",
			InputPerMillion: 18, OutputPerMillion: 72,
			CacheReadPerMillion: 3.6, CacheWritePerMillion: 18,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		{
			Provider: "OpenAI", Model: "gpt-4o-mini",
			InputPerMillion: 1.1, OutputPerMillion: 4.4,
			CacheReadPerMillion: 0.22, CacheWritePerMillion: 1.1,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		// Moonshot
		{
			Provider: "Moonshot", Model: "kimi-k2",
			InputPerMillion: 4, OutputPerMillion: 16,
			CacheReadPerMillion: 1, CacheWritePerMillion: 4,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		{
			Provider: "Moonshot", Model: "moonshot-v1-8k",
			InputPerMillion: 12, OutputPerMillion: 12,
			CacheReadPerMillion: 2.4, CacheWritePerMillion: 12,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		// Zhipu
		{
			Provider: "Zhipu", Model: "glm-4-plus",
			InputPerMillion: 50, OutputPerMillion: 50,
			CacheReadPerMillion: 5, CacheWritePerMillion: 50,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		{
			Provider: "Zhipu", Model: "glm-4-flash",
			InputPerMillion: 0, OutputPerMillion: 0,
			CacheReadPerMillion: 0, CacheWritePerMillion: 0,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		// Qwen
		{
			Provider: "Qwen", Model: "qwen-max",
			InputPerMillion: 20, OutputPerMillion: 60,
			CacheReadPerMillion: 4, CacheWritePerMillion: 20,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		{
			Provider: "Qwen", Model: "qwen-plus",
			InputPerMillion: 4, OutputPerMillion: 12,
			CacheReadPerMillion: 0.8, CacheWritePerMillion: 4,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		// Ollama 本地模型（价格 0）
		{
			Provider: "Ollama", Model: "llama3",
			InputPerMillion: 0, OutputPerMillion: 0,
			CacheReadPerMillion: 0, CacheWritePerMillion: 0,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		{
			Provider: "Ollama", Model: "qwen2.5",
			InputPerMillion: 0, OutputPerMillion: 0,
			CacheReadPerMillion: 0, CacheWritePerMillion: 0,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
		{
			Provider: "Ollama", Model: "deepseek-r1",
			InputPerMillion: 0, OutputPerMillion: 0,
			CacheReadPerMillion: 0, CacheWritePerMillion: 0,
			Currency: "CNY", PlanType: PlanTypePayAsYouGo,
		},
	}
}
