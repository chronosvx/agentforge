// Package cost 提供基于 token 用量估算成本与预算管理能力。
//
// DSH 真实事件流当前不直接发出 cost projection，而是发出 tokenUsage；
// 本包把 token 用量按可配置的每百万 token 单价换算为估算成本（元），
// 并提供 Budget 用于按会话/项目维护预算与告警判断。
package cost

import (
	"encoding/json"
	"math"
)

// 默认单价（元 / 百万 tokens）。
// 参考项目内 mock 与 DeepSeek 常见价格：
//   - 未命中输入：1.5 元/百万
//   - 输出：4.5 元/百万
//   - 缓存读取：0.05 元/百万
//   - 缓存写入：1.5 元/百万（无单独报价时按未命中输入保守估算）
const (
	DefaultInputPerMillion      = 1.5
	DefaultOutputPerMillion     = 4.5
	DefaultCacheReadPerMillion  = 0.05
	DefaultCacheWritePerMillion = 1.5
)

// Estimator 根据 token 用量估算成本。
// 所有单价均以“元 / 百万 tokens”为单位。
type Estimator struct {
	InputPerMillion      float64 // 未命中输入 token 单价
	OutputPerMillion     float64 // 输出 token 单价
	CacheReadPerMillion  float64 // 缓存读取 token 单价
	CacheWritePerMillion float64 // 缓存写入 token 单价
}

// NewEstimator 返回使用默认单价的估算器。
func NewEstimator() *Estimator {
	return &Estimator{
		InputPerMillion:      DefaultInputPerMillion,
		OutputPerMillion:     DefaultOutputPerMillion,
		CacheReadPerMillion:  DefaultCacheReadPerMillion,
		CacheWritePerMillion: DefaultCacheWritePerMillion,
	}
}

// NewCustomEstimator 返回使用自定义单价的估算器。
func NewCustomEstimator(inputPerMillion, outputPerMillion, cacheReadPerMillion, cacheWritePerMillion float64) *Estimator {
	return &Estimator{
		InputPerMillion:      inputPerMillion,
		OutputPerMillion:     outputPerMillion,
		CacheReadPerMillion:  cacheReadPerMillion,
		CacheWritePerMillion: cacheWritePerMillion,
	}
}

// Estimate 根据四类 token 数量计算估算成本（元）。
// 公式：input/1e6*单价 + output/1e6*单价 + cacheRead/1e6*单价 + cacheWrite/1e6*单价。
func (e *Estimator) Estimate(inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) float64 {
	if e == nil {
		e = NewEstimator()
	}
	return float64(inputTokens)*e.InputPerMillion/1e6 +
		float64(outputTokens)*e.OutputPerMillion/1e6 +
		float64(cacheReadTokens)*e.CacheReadPerMillion/1e6 +
		float64(cacheWriteTokens)*e.CacheWritePerMillion/1e6
}

// EstimateFromUsage 从 tokenUsage map 中提取字段并估算成本。
// 兼容 DSH tokenUsage 的常见字段名：
//   - uncachedInputTokens / inputTokens / uncachedInput / input
//   - outputTokens / output
//   - cacheReadTokens / cacheReadInputTokens / cacheHitTokens / cacheHits
//   - cacheWriteTokens / cacheWrite
func (e *Estimator) EstimateFromUsage(usage map[string]any) float64 {
	if e == nil {
		e = NewEstimator()
	}
	return e.Estimate(
		intFromUsage(usage, "uncachedInputTokens", "inputTokens", "uncachedInput", "input"),
		intFromUsage(usage, "outputTokens", "output"),
		intFromUsage(usage, "cacheReadTokens", "cacheReadInputTokens", "cacheHitTokens", "cacheHits"),
		intFromUsage(usage, "cacheWriteTokens", "cacheWrite"),
	)
}

// EstimateFromUsage 使用默认单价从 tokenUsage map 估算成本。
func EstimateFromUsage(usage map[string]any) float64 {
	return NewEstimator().EstimateFromUsage(usage)
}

// EstimateFromUsageWithPricing 根据厂商定价从 tokenUsage map 估算成本。
//
// 按量付费（pay_as_you_go）时使用 pricing 中的每百万 token 单价计算；
// Token Plan（token_plan）时简单返回 pricing.MonthlyFee 作为包月固定成本。
// 返回值的货币单位与 pricing.Currency 一致。
func EstimateFromUsageWithPricing(usage map[string]any, pricing Pricing) float64 {
	if pricing.PlanType == PlanTypeTokenPlan {
		return pricing.MonthlyFee
	}
	e := NewCustomEstimator(
		pricing.InputPerMillion,
		pricing.OutputPerMillion,
		pricing.CacheReadPerMillion,
		pricing.CacheWritePerMillion,
	)
	return e.EstimateFromUsage(usage)
}

// Budget 表示一个会话或项目的成本预算。
// Total 是预算总额，Used 是已用金额，Remaining 为 Total - Used。
// 推荐通过 NewBudget 创建；Add 会自动维护 Remaining。
type Budget struct {
	Total     float64
	Used      float64
	Remaining float64
}

// NewBudget 创建一个总额为 total 的预算。
func NewBudget(total float64) *Budget {
	return &Budget{Total: total, Remaining: total}
}

// Add 增加一笔已用金额，并同步更新剩余预算。
func (b *Budget) Add(amount float64) {
	if b == nil {
		return
	}
	b.Used += amount
	b.Remaining = b.Total - b.Used
}

// Exceeded 报告是否已超支（Used > Total）。
func (b *Budget) Exceeded() bool {
	if b == nil {
		return false
	}
	return b.Used > b.Total
}

// intFromUsage 从 usage 中按候选 key 顺序读取第一个数值，转为 int64。
// 支持 JSON 反序列化常见的 float64，以及 int/int64/uint64/json.Number 等类型。
func intFromUsage(usage map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v, ok := usage[key]; ok {
			if n, ok := toInt64(v); ok {
				return n
			}
		}
	}
	return 0
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n <= math.MaxInt64 {
			return int64(n), true
		}
		return 0, false
	case float32:
		return int64(n), true
	case float64:
		return int64(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		if f, err := n.Float64(); err == nil {
			return int64(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}
