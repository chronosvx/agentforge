package session

// 此文件提供本地会话历史消息的简单全文索引。
//
// 当前实现为内存索引：Add/AddAll 将会话消息载入内存，Search 对消息内容
// 做大小写不敏感的子串匹配，并返回命中消息及其前后文摘要。

import (
	"strings"

	"github.com/chronosvx/agentforge/internal/client"
)

// Result 表示一次消息搜索命中的位置与摘要。
type Result struct {
	SessionID    string
	MessageIndex int
	Role         string
	Content      string
	Snippet      string
}

// Index 是内存中的历史消息搜索索引。
//
// 零值可以直接使用。Add 对同一会话 ID 会替换旧消息，保持首次加入时的顺序。
type Index struct {
	messages map[string][]client.Message
	lower    map[string][]string // 每条消息的小写副本缓存，Search 首次访问时惰性构建。
	order    []string
}

// Add 将单个会话的消息加入索引。同一会话 ID 再次添加时使用最新消息替换旧内容。
//
// Add 只更新对应会话并失效该会话的小写缓存，不会重建整个索引，因此适合在
// 大会话场景下增量更新。
func (idx *Index) Add(entry Entry) {
	if idx.messages == nil {
		idx.messages = make(map[string][]client.Message)
	}
	if _, ok := idx.messages[entry.ID]; !ok {
		idx.order = append(idx.order, entry.ID)
	}
	idx.messages[entry.ID] = append([]client.Message(nil), entry.Messages...)
	// 消息已变化，使该会话的小写缓存失效；Search 首次访问时再重建。
	if idx.lower != nil {
		delete(idx.lower, entry.ID)
	}
}

// AddAll 批量加入多个会话。按传入顺序保留会话的索引顺序。
func (idx *Index) AddAll(entries []Entry) {
	for _, entry := range entries {
		idx.Add(entry)
	}
}

// AddAsync 是 Add 的别名，明确表达“增量加入、不重建整个索引”的语义。
//
// 当前搜索索引为纯内存结构，Add/AddAsync 都只更新单个会话并失效对应的小写
// 缓存，不会全量重建；调用方可在大会话场景下放心增量更新。
func (idx *Index) AddAsync(entry Entry) {
	idx.Add(entry)
}

// Search 在全部已索引消息中查找 query，返回大小写不敏感的子串匹配结果。
// 空查询返回 nil。limit 大于 0 时最多返回 limit 条结果；limit <= 0 表示不限制。
// 结果按会话加入顺序、消息在会话中的顺序排列。
func (idx *Index) Search(query string, limit int) []Result {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	lowerQuery := strings.ToLower(query)
	if idx.lower == nil {
		idx.lower = make(map[string][]string)
	}
	var results []Result
	for _, id := range idx.order {
		msgs := idx.messages[id]
		lowers := idx.lower[id]
		if len(lowers) != len(msgs) {
			lowers = make([]string, len(msgs))
			for i, msg := range msgs {
				lowers[i] = strings.ToLower(msg.Content)
			}
			idx.lower[id] = lowers
		}
		for i, msg := range msgs {
			if !strings.Contains(lowers[i], lowerQuery) {
				continue
			}
			results = append(results, Result{
				SessionID:    id,
				MessageIndex: i,
				Role:         msg.Role,
				Content:      msg.Content,
				Snippet:      makeSnippet(msg.Content, query),
			})
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// snippetRadius 是命中词前后各保留的 rune 数量。
const snippetRadius = 40

// makeSnippet 围绕第一个命中位置生成带前后文的摘要，过长时用省略号截断。
func makeSnippet(content, query string) string {
	lowerContent := []rune(strings.ToLower(content))
	lowerQuery := []rune(strings.ToLower(query))
	pos := indexRunes(lowerContent, lowerQuery)
	if pos < 0 {
		return ellipsize(content, snippetRadius*2)
	}

	runes := []rune(content)
	start := pos - snippetRadius
	if start < 0 {
		start = 0
	}
	end := pos + len(lowerQuery) + snippetRadius
	if end > len(runes) {
		end = len(runes)
	}

	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	b.WriteString(string(runes[start:end]))
	if end < len(runes) {
		b.WriteString("…")
	}
	return b.String()
}

// indexRunes 返回 needle 在 hay 中第一次出现的起始下标；未找到时返回 -1。
func indexRunes(hay, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		matched := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

// ellipsize 将 runes 截断到 max 长度，并在截断处添加省略号。
func ellipsize(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 0 {
		return "…"
	}
	half := max / 2
	return string(runes[:half]) + "…" + string(runes[len(runes)-half:])
}
