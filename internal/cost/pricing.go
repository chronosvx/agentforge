package cost

// PlanType 表示计费模式。
type PlanType string

const (
	// PlanTypePayAsYouGo 表示按量付费（按 token 用量计费）。
	PlanTypePayAsYouGo PlanType = "pay_as_you_go"
	// PlanTypeTokenPlan 表示 Token Plan / 包月套餐（按固定月费计费）。
	PlanTypeTokenPlan PlanType = "token_plan"
)

// Pricing 描述一个厂商/模型的动态成本单价。
//
// 所有 PerMillion 单价均以“货币单位 / 百万 tokens”为单位；
// MonthlyFee 仅在 PlanType == token_plan 时有意义，表示包月固定成本。
// 该类型同时通过 internal/provider 的别名暴露为 provider.Pricing。
type Pricing struct {
	Provider             string
	Model                string
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
	Currency             string
	PlanType             PlanType
	MonthlyFee           float64
}
