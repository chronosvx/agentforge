// Package contextx 提供长会话上下文窗口管理、压缩摘要与 Checkpoint/Rewind。
//
// 包名刻意使用 contextx 而不是 context，避免与标准库 context 冲突。
// 当前实现不依赖具体 UI，可被 session、provider 或后续 UI 适配层复用。
package contextx

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chronosvx/agentforge/internal/client"
)

// Message 是上下文窗口中的一条消息。
//
// 复用 client.Message，便于直接与现有会话存储/Channel 客户端互通。
type Message = client.Message

// TokenEstimator 根据文本估算 token 数。
//
// 这里使用简单启发式：约 4 个字符/1 token。后续可替换为按模型加载的
// tokenizer（如 tiktoken 或厂商 API 返回的 usage），无需改动 Window 对外接口。
type TokenEstimator func(text string) int64

// DefaultTokenEstimator 返回默认的启发式 token 估算器。
func DefaultTokenEstimator(text string) int64 {
	n := int64(utf8.RuneCountInString(text))
	return (n + 3) / 4
}

// messageOverheadTokens 是每条消息额外计入的固定开销（角色、分隔符等）。
const messageOverheadTokens = 4

// Window 管理消息列表与 token 估算。
//
// Window 是并发安全的：Add/TrimToLimit/Rewind 等写操作会加锁，
// Messages/TokenCount 等读操作返回快照，避免调用方与内部状态互相干扰。
type Window struct {
	mu       sync.RWMutex
	messages []Message
	estimate TokenEstimator
}

// NewWindow 返回一个使用默认 token 估算器的空窗口。
func NewWindow() *Window {
	return NewWindowWithEstimator(DefaultTokenEstimator)
}

// NewWindowWithEstimator 返回一个使用自定义 token 估算器的空窗口。
func NewWindowWithEstimator(estimate TokenEstimator) *Window {
	if estimate == nil {
		estimate = DefaultTokenEstimator
	}
	return &Window{estimate: estimate}
}

// Add 追加一条消息到窗口末尾。
func (w *Window) Add(message Message) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = append(w.messages, message)
}

// Messages 返回当前消息列表的副本。
func (w *Window) Messages() []Message {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]Message(nil), w.messages...)
}

// Len 返回当前消息条数。
func (w *Window) Len() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.messages)
}

// TokenCount 返回当前窗口的估算 token 总数。
func (w *Window) TokenCount() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.countTokens(w.messages)
}

// TrimToLimit 从窗口头部裁剪最旧消息，直到总 token 数不超过 limit。
//
// 至少保留最新一条消息：即使最新消息本身超过 limit，也不会把窗口清空。
// 返回值是被裁剪掉的旧消息，便于调用方对它们做摘要或归档。
func (w *Window) TrimToLimit(limit int64) []Message {
	w.mu.Lock()
	defer w.mu.Unlock()

	if limit < 0 {
		limit = 0
	}

	var removed []Message
	for w.countTokens(w.messages) > limit && len(w.messages) > 1 {
		removed = append(removed, w.messages[0])
		w.messages = w.messages[1:]
	}
	return removed
}

// Checkpoint 保存当前窗口的不可变快照。
func (w *Window) Checkpoint(id string) Checkpoint {
	w.mu.RLock()
	defer w.mu.RUnlock()
	msgs := append([]Message(nil), w.messages...)
	return Checkpoint{
		ID:         id,
		CreatedAt:  time.Now(),
		Messages:   msgs,
		TokenCount: w.countTokens(msgs),
	}
}

// Rewind 将窗口恢复到指定 checkpoint 中的消息快照。
func (w *Window) Rewind(cp Checkpoint) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = append([]Message(nil), cp.Messages...)
}

// Compact 使用 summarizer 压缩旧消息，以降低窗口 token 占用。
//
// 当窗口未超过 limit 时不做任何事。超过 limit 时，它会将除最新一条外的
// 旧消息交给 summarizer 生成摘要，并用一条 system 摘要消息替换它们；
// 若摘要后仍超过 limit，则回退为 TrimToLimit 的简单裁剪。
// 返回被压缩/裁剪掉的旧消息。
func (w *Window) Compact(limit int64, summarizer Summarizer) ([]Message, error) {
	if summarizer == nil {
		return nil, errors.New("contextx: nil summarizer")
	}
	if w.TokenCount() <= limit {
		return nil, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.messages) <= 1 {
		return nil, nil
	}

	old := append([]Message(nil), w.messages[:len(w.messages)-1]...)
	newest := w.messages[len(w.messages)-1]

	summary, err := summarizer.Summarize(old)
	if err != nil {
		return nil, err
	}
	summaryMsg := summary.AsMessage("system")
	compacted := append([]Message{summaryMsg}, newest)

	if w.countTokens(compacted) <= limit {
		w.messages = compacted
		return old, nil
	}

	// 摘要仍超限时回退到简单裁剪，至少保留最新一条。
	original := w.messages
	w.messages = append([]Message(nil), original...)
	return w.trimLocked(limit), nil
}

// countTokens 计算给定消息列表的估算 token 数。
// 调用方需持有锁。
func (w *Window) countTokens(messages []Message) int64 {
	estimate := w.estimate
	if estimate == nil {
		estimate = DefaultTokenEstimator
	}
	var total int64
	for _, msg := range messages {
		total += messageOverheadTokens + estimate(msg.Role) + estimate(msg.Content)
	}
	return total
}

// trimLocked 在已持有锁的前提下执行 TrimToLimit 的裁剪逻辑。
func (w *Window) trimLocked(limit int64) []Message {
	if limit < 0 {
		limit = 0
	}
	var removed []Message
	for w.countTokens(w.messages) > limit && len(w.messages) > 1 {
		removed = append(removed, w.messages[0])
		w.messages = w.messages[1:]
	}
	return removed
}

// Summary 是压缩摘要的结果。
//
// Text 保存摘要文本；TokenCount 是摘要自身的估算 token 数；CreatedAt 记录生成时间；
// Meta 可携带 LLM 压缩接口返回的附加信息（模型名、压缩策略等）。
type Summary struct {
	Text       string         `json:"text"`
	TokenCount int64          `json:"tokenCount"`
	CreatedAt  time.Time      `json:"createdAt,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
}

// Summarizer 定义将一组旧消息压缩为摘要的接口。
//
// 当前内置 SimpleSummarizer；后续可接入 LLM 压缩（例如调用 provider.Provider）
// 时实现该接口即可，Window 无需感知具体实现。
type Summarizer interface {
	Summarize(messages []Message) (Summary, error)
}

// SummarizeFunc 是 Summarizer 的函数式实现。
type SummarizeFunc func(messages []Message) (Summary, error)

// Summarize 实现 Summarizer 接口。
func (f SummarizeFunc) Summarize(messages []Message) (Summary, error) {
	return f(messages)
}

// SimpleSummarizer 是简单的本地摘要器：拼接旧消息并在超过 maxTokens 时截断。
type SimpleSummarizer struct {
	MaxTokens int64
}

// Summarize 实现 Summarizer 接口。
func (s SimpleSummarizer) Summarize(messages []Message) (Summary, error) {
	return SummarizeMessages(messages, s.MaxTokens), nil
}

// SummarizeMessages 将旧消息拼接为摘要文本，并在超过 maxTokens 时按字符粗略截断。
//
// maxTokens <= 0 表示不截断。该函数是“可简单截断/拼接”的默认实现，
// 也是接入 LLM 压缩接口前的占位实现。
func SummarizeMessages(messages []Message, maxTokens int64) Summary {
	var b strings.Builder
	for i, msg := range messages {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Content)
	}
	text := b.String()
	tokens := DefaultTokenEstimator(text)

	if maxTokens > 0 && tokens > maxTokens {
		// 粗略按 4 字符/token 截断，并追加截断标记。
		runes := []rune(text)
		maxRunes := int(maxTokens * 4)
		if maxRunes < len(runes) {
			text = string(runes[:maxRunes]) + "\n…[truncated]"
			tokens = DefaultTokenEstimator(text)
		}
	}

	return Summary{
		Text:       text,
		TokenCount: tokens,
		CreatedAt:  time.Now(),
	}
}

// AsMessage 将摘要转换为可放回 Window 的消息。
// role 为空时默认使用 "system"。
func (s Summary) AsMessage(role string) Message {
	if role == "" {
		role = "system"
	}
	return Message{Role: role, Content: s.Text}
}
