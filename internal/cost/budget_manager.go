package cost

import (
	"strings"
	"sync"
)

// BudgetManager 按 provider/model 维度维护多个独立预算。
//
// BudgetManager 是线程安全的，可被多个会话或请求并发调用。
// Summary 的 key 使用规范化的 "provider/model"（小写、去除首尾空白）。
type BudgetManager struct {
	mu      sync.RWMutex
	budgets map[string]*Budget
}

// NewBudgetManager 返回空的预算管理器。
func NewBudgetManager() *BudgetManager {
	return &BudgetManager{budgets: make(map[string]*Budget)}
}

// AddBudget 设置（或覆盖）指定 provider/model 的预算总额。
// 已用金额会随新预算重置为 0。
func (m *BudgetManager) AddBudget(provider, model string, total float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLocked()
	m.budgets[budgetManagerKey(provider, model)] = NewBudget(total)
}

// Spend 在指定 provider/model 的预算上增加一笔已用金额。
// 如果该维度尚未 AddBudget，会按总额 0 创建预算，以便 Remaining 反映超支。
func (m *BudgetManager) Spend(provider, model string, amount float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLocked()

	key := budgetManagerKey(provider, model)
	b := m.budgets[key]
	if b == nil {
		b = NewBudget(0)
		m.budgets[key] = b
	}
	b.Add(amount)
}

// Remaining 返回指定 provider/model 的剩余预算。
// 未设置预算时返回 0。
func (m *BudgetManager) Remaining(provider, model string) float64 {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if b := m.budgets[budgetManagerKey(provider, model)]; b != nil {
		return b.Remaining
	}
	return 0
}

// Summary 返回当前所有 provider/model 的剩余预算快照。
// key 为 "provider/model"，value 为该维度剩余金额。
func (m *BudgetManager) Summary() map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]float64, len(m.budgets))
	for key, b := range m.budgets {
		out[key] = b.Remaining
	}
	return out
}

func (m *BudgetManager) ensureLocked() {
	if m.budgets == nil {
		m.budgets = make(map[string]*Budget)
	}
}

func budgetManagerKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.ToLower(strings.TrimSpace(model))
}
