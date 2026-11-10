package retrieval

import (
	"math"
	"sort"
	"sync"
)

// Doc 是倒排索引里的一条可检索切片。正文与结构路径都取自数据库，
// 索引常驻进程，提问时不再回表读取全班切片。
type Doc struct {
	ID            int64
	MaterialID    int64
	Seq           int
	Text          string
	HeadingPath   string
	MaterialTitle string
}

// Index 是进程内的 BM25 倒排索引。
//
// 建索引只在首次提问或材料变更后发生一次；查询只访问命中词元的 posting list，
// 不对全部切片重新分词、不统计全语料 DF、不做全量排序。
type Index struct {
	mu       sync.RWMutex
	docs     map[int64]Doc
	body     map[string]map[int64]int // 词元 -> 切片 ID -> 词频
	head     map[string]map[int64]int // 结构路径词元 -> 切片 ID -> 词频
	length   map[int64]int            // 切片正文词元数
	byMatSeq map[int64]map[int]int64  // materialID -> seq -> 切片 ID
	totalLen int
}

// NewIndex 构造空索引。
func NewIndex() *Index { return &Index{} }

// Build 用一批切片整体重建索引（材料变更后由调用方触发一次）。
func (ix *Index) Build(docs []Doc) {
	body := make(map[string]map[int64]int)
	head := make(map[string]map[int64]int)
	length := make(map[int64]int, len(docs))
	byMatSeq := make(map[int64]map[int]int64)
	dm := make(map[int64]Doc, len(docs))
	total := 0
	for _, d := range docs {
		dm[d.ID] = d
		n := 0
		for _, t := range Tokenize(d.Text) {
			if !Informative(t.Text) {
				continue
			}
			n++
			if body[t.Text] == nil {
				body[t.Text] = make(map[int64]int)
			}
			body[t.Text][d.ID]++
		}
		for _, t := range Tokenize(d.HeadingPath) {
			if !Informative(t.Text) {
				continue
			}
			if head[t.Text] == nil {
				head[t.Text] = make(map[int64]int)
			}
			head[t.Text][d.ID]++
		}
		length[d.ID] = n
		total += n
		if byMatSeq[d.MaterialID] == nil {
			byMatSeq[d.MaterialID] = make(map[int]int64)
		}
		byMatSeq[d.MaterialID][d.Seq] = d.ID
	}
	ix.mu.Lock()
	ix.docs, ix.body, ix.head = dm, body, head
	ix.length, ix.byMatSeq, ix.totalLen = length, byMatSeq, total
	ix.mu.Unlock()
}

// Len 返回索引中的切片数。
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.docs)
}

// Doc 按 ID 取切片元数据。
func (ix *Index) Doc(id int64) (Doc, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	d, ok := ix.docs[id]
	return d, ok
}

// Search 按标准 BM25 打分，只访问命中词元的 posting list。
// 返回按分数降序的切片 ID（同分按 ID 升序，保证可复现）。
func (ix *Index) Search(terms []string, limit int) []int64 {
	if limit <= 0 || len(terms) == 0 {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	n := float64(len(ix.docs))
	if n == 0 {
		return nil
	}
	avgdl := float64(ix.totalLen) / n
	if avgdl <= 0 {
		avgdl = 1
	}
	const (
		k1 = 1.2
		b  = 0.75
	)
	score := make(map[int64]float64)
	seen := make(map[string]bool)
	for _, term := range terms {
		if seen[term] {
			continue
		}
		seen[term] = true
		post := ix.body[term]
		df := float64(len(post))
		if df == 0 {
			continue
		}
		idf := 1 + math.Log2(1+(n-df+0.5)/(df+0.5))
		for id, tf := range post {
			dl := float64(ix.length[id])
			denom := float64(tf) + k1*(1-b+b*dl/avgdl)
			score[id] += idf * float64(tf) * (k1 + 1) / denom
		}
	}
	return rankIDs(score, limit)
}

// SearchHeading 按结构路径命中的不同词元数排序，为「酶是什么」这类
// 只含标题词的短问句提供一条确定入口。
func (ix *Index) SearchHeading(terms []string, limit int) []int64 {
	if limit <= 0 || len(terms) == 0 {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	score := make(map[int64]float64)
	seen := make(map[string]bool)
	for _, term := range terms {
		if seen[term] {
			continue
		}
		seen[term] = true
		for id := range ix.head[term] {
			score[id]++
		}
	}
	return rankIDs(score, limit)
}

// Neighbours 返回同一材料内序号相邻的切片 ID，用于给命中片补充语境。
func (ix *Index) Neighbours(materialID int64, seq int) []int64 {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	m := ix.byMatSeq[materialID]
	if m == nil {
		return nil
	}
	var out []int64
	for _, s := range []int{seq - 1, seq + 1} {
		if id, ok := m[s]; ok {
			out = append(out, id)
		}
	}
	return out
}

func rankIDs(score map[int64]float64, limit int) []int64 {
	ids := make([]int64, 0, len(score))
	for id, s := range score {
		if s > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] < ids[j]
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}
