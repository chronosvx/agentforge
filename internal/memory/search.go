package memory

import (
	"context"
	"strings"
)

// Embedder 预留的本地 embedding 接口。
//
// 后续接入本地向量模型后，可实现该接口并将向量写入/检索流程接入 Searcher；
// 当前检索仍使用关键词/子串匹配，不依赖外部服务。
type Embedder interface {
	// Embed 将一批文本转换为等长向量。实现方应保证返回的向量长度一致。
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Searcher 基于 MemoryStore 提供项目记忆的关键词/子串本地检索。
//
// 零值不可直接使用；请通过 NewSearcher 创建。
type Searcher struct {
	store *MemoryStore
}

// NewSearcher 创建一个绑定到指定 MemoryStore 的检索器。
func NewSearcher(store *MemoryStore) *Searcher {
	return &Searcher{store: store}
}

// Search 在指定项目的记忆条目中检索 query，返回大小写不敏感的关键词匹配结果。
//
// 查询会按空白拆分为多个关键词，条目需要在 Content、Key 或 Tags 中同时包含
// 全部关键词才算命中。空查询返回 nil。limit 大于 0 时最多返回 limit 条；
// limit <= 0 表示不限制。结果按 MemoryStore.List 的排序返回。
func (s *Searcher) Search(project, query string, limit int) []Entry {
	if s == nil || s.store == nil {
		return nil
	}
	entries, err := s.store.List(context.Background(), project)
	if err != nil {
		return nil
	}
	return SearchEntries(entries, query, limit)
}

// SearchEntries 在给定条目集合中执行关键词/子串检索，供不依赖 MemoryStore
// 的场景直接使用。
func SearchEntries(entries []Entry, query string, limit int) []Entry {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	terms := strings.Fields(strings.ToLower(query))
	var results []Entry
	for _, entry := range entries {
		if matches(entry, terms) {
			results = append(results, entry)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results
}

func matches(entry Entry, terms []string) bool {
	haystack := strings.ToLower(entry.Key + "\n" + entry.Content)
	for _, tag := range entry.Tags {
		haystack += "\n" + strings.ToLower(tag)
	}
	for _, term := range terms {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}
