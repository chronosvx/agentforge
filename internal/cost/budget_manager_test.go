package cost

import (
	"sync"
	"testing"
)

func TestBudgetManagerPerProviderModel(t *testing.T) {
	m := NewBudgetManager()
	m.AddBudget("openai", "gpt-4o-mini", 10)
	m.AddBudget("deepseek", "deepseek-chat", 20)

	if got := m.Remaining("openai", "gpt-4o-mini"); !almostEqual(got, 10) {
		t.Fatalf("Remaining(openai/gpt-4o-mini) = %v, want 10", got)
	}
	if got := m.Remaining("deepseek", "deepseek-chat"); !almostEqual(got, 20) {
		t.Fatalf("Remaining(deepseek/deepseek-chat) = %v, want 20", got)
	}

	m.Spend("openai", "gpt-4o-mini", 4)
	m.Spend("deepseek", "deepseek-chat", 25)

	if got := m.Remaining("openai", "gpt-4o-mini"); !almostEqual(got, 6) {
		t.Fatalf("Remaining(openai/gpt-4o-mini) after spend = %v, want 6", got)
	}
	if got := m.Remaining("deepseek", "deepseek-chat"); !almostEqual(got, -5) {
		t.Fatalf("Remaining(deepseek/deepseek-chat) after overspend = %v, want -5", got)
	}
}

func TestBudgetManagerCaseInsensitiveKey(t *testing.T) {
	m := NewBudgetManager()
	m.AddBudget("OpenAI", "GPT-4o-mini", 10)
	m.Spend("openai", "gpt-4o-mini", 3)

	if got := m.Remaining("OPENAI", "GPT-4O-MINI"); !almostEqual(got, 7) {
		t.Fatalf("Remaining(case-insensitive) = %v, want 7", got)
	}
}

func TestBudgetManagerSummary(t *testing.T) {
	m := NewBudgetManager()
	m.AddBudget("openai", "gpt-4o-mini", 10)
	m.AddBudget("deepseek", "deepseek-chat", 20)
	m.Spend("openai", "gpt-4o-mini", 4)

	summary := m.Summary()
	if len(summary) != 2 {
		t.Fatalf("Summary() len = %d, want 2: %#v", len(summary), summary)
	}
	if !almostEqual(summary["openai/gpt-4o-mini"], 6) {
		t.Fatalf("Summary()[openai/gpt-4o-mini] = %v, want 6", summary["openai/gpt-4o-mini"])
	}
	if !almostEqual(summary["deepseek/deepseek-chat"], 20) {
		t.Fatalf("Summary()[deepseek/deepseek-chat] = %v, want 20", summary["deepseek/deepseek-chat"])
	}
}

func TestBudgetManagerSpendWithoutBudget(t *testing.T) {
	m := NewBudgetManager()
	m.Spend("unknown", "model", 5)

	if got := m.Remaining("unknown", "model"); !almostEqual(got, -5) {
		t.Fatalf("Remaining(unknown/model) after spend without budget = %v, want -5", got)
	}
}

func TestBudgetManagerConcurrentSpend(t *testing.T) {
	m := NewBudgetManager()
	m.AddBudget("openai", "gpt-4o-mini", 100)

	const workers = 8
	const opsPerWorker = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				m.Spend("openai", "gpt-4o-mini", 0.01)
			}
		}()
	}
	wg.Wait()

	if got := m.Remaining("openai", "gpt-4o-mini"); !almostEqual(got, 100-float64(workers*opsPerWorker)*0.01) {
		t.Fatalf("Remaining after concurrent spend = %v, want %v", got, 100-float64(workers*opsPerWorker)*0.01)
	}
}
