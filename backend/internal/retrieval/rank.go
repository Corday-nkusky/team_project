package retrieval

import (
	"sort"
	"strings"
)

// RankK 是 RRF 的常数：分数按名次计算，与召回路的原始分量纲无关。
const RankK = 60

// FuseRRF 用倒数排名融合把多路有序候选合成一路。
//
// 每路只提供名次，分数为 Σ 1/(k+rank)；同名次按 ID 升序，保证结果可复现。
func FuseRRF(lists [][]int64, limit int) []int64 {
	score := make(map[int64]float64)
	best := make(map[int64]int)
	for _, list := range lists {
		seen := make(map[int64]bool)
		rank := 0
		for _, id := range list {
			if seen[id] {
				continue
			}
			seen[id] = true
			rank++
			score[id] += 1.0 / float64(RankK+rank)
			if b, ok := best[id]; !ok || rank < b {
				best[id] = rank
			}
		}
	}
	ids := make([]int64, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		if best[ids[i]] != best[ids[j]] {
			return best[ids[i]] < best[ids[j]]
		}
		return ids[i] < ids[j]
	})
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

// Candidate 是进入本地二阶段精排的一条候选。
type Candidate struct {
	ID       int64
	Doc      Doc
	VecScore float64 // 语义路最高余弦分；无向量时为 0
	RRFRank  int

	// 以下由 Rerank 计算。
	Hits       int
	Coverage   float64
	Window     int
	HeadingHit bool
	Evidence   bool

	// terms 缓存词元集合，供去冗余复用，避免二次全文分词。
	terms map[string]bool
}

// EvidencePolicy 是「有依据」的判定参数，沿用既有配置项，不新增阈值。
//
// 向量分作用在**查询级**：QueryVecScore 是本班候选里的最高余弦分，用来判断
// 「这个问题和本班材料是否相关」；片段级依据由词元覆盖窗口与结构路径命中决定。
// 这样向量不会被回填成逐片段的加权分，也不会让任何最近邻都显得"有依据"。
type EvidencePolicy struct {
	MinCoverage   float64
	MinVecScore   float64
	VecEnabled    bool
	QueryVecScore float64
}

// TightWindow 是词元集中出现在相邻句内的窗口上限（rune）。
const TightWindow = 200

// Rerank 计算候选证据并做可判定的分层排序，返回按优劣排列的候选。
//
// 排序不用跨量纲加权和：结构命中优先 → 词元邻近档位 → 覆盖率 → RRF 名次
// → 余弦分 → ID。调用方只取 Evidence 为 true 的条目。
func Rerank(cands []Candidate, terms []string, p EvidencePolicy) []Candidate {
	out := make([]Candidate, len(cands))
	copy(out, cands)
	for i := range out {
		c := &out[i]
		// 一次分词同时产出词元集合与覆盖窗口：集合供去冗余复用，
		// 窗口供证据判定与排序，不再对同一段正文扫描两遍。
		c.terms, c.Window, c.Hits = analyze(c.Doc.Text, terms)
		c.Coverage = Coverage(c.Hits, len(terms))
		for _, t := range terms {
			if strings.Contains(c.Doc.HeadingPath, t) {
				c.HeadingHit = true
				break
			}
		}
		// 词面证据：结构路径命中，或查询词集中在相邻窗口且覆盖率达标。
		lexical := len(terms) > 0 &&
			(c.HeadingHit || (c.Coverage >= p.MinCoverage && c.Window <= TightWindow))
		if p.VecEnabled {
			// 语义路可用：先要求「本班确有与问题相关的内容」，再要求该片段
			// 自身有词面证据或语义证据。Qdrant 的最近邻分数非零不能单独放行。
			c.Evidence = p.QueryVecScore >= p.MinVecScore &&
				(lexical || c.VecScore >= p.MinVecScore)
		} else {
			// 零密钥降级：没有语义分，词面证据需更实（多词元命中或标题命中），
			// 避免单个偶然子串把无关材料当成依据。
			c.Evidence = lexical && (c.Hits >= 2 || c.HeadingHit)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Evidence != b.Evidence {
			return a.Evidence
		}
		if a.HeadingHit != b.HeadingHit {
			return a.HeadingHit
		}
		if windowBand(a.Window) != windowBand(b.Window) {
			return windowBand(a.Window) < windowBand(b.Window)
		}
		if a.Coverage != b.Coverage {
			return a.Coverage > b.Coverage
		}
		if a.RRFRank != b.RRFRank {
			return a.RRFRank < b.RRFRank
		}
		if a.VecScore != b.VecScore {
			return a.VecScore > b.VecScore
		}
		return a.ID < b.ID
	})
	return out
}

// Deduplicate 按边际相关性剔除高度重叠的候选：
// 与已选条目同材料且词元集合 Jaccard 超阈值的跳过，使 top-k 覆盖不同方面。
func Deduplicate(ordered []Candidate, limit int, threshold float64) []Candidate {
	var out []Candidate
	for i := range ordered {
		c := &ordered[i]
		if limit > 0 && len(out) >= limit {
			break
		}
		// 精排已经算过词元集合；只有外部直接构造的候选才需要补算一次。
		if c.terms == nil {
			c.terms = termSet(c.Doc.Text)
		}
		redundant := false
		for j := range out {
			prev := &out[j]
			if prev.Doc.MaterialID != c.Doc.MaterialID {
				continue
			}
			if jaccard(prev.terms, c.terms) >= threshold {
				redundant = true
				break
			}
		}
		if redundant {
			continue
		}
		out = append(out, *c)
	}
	return out
}

func windowBand(w int) int {
	switch {
	case w <= 24:
		return 0
	case w <= 80:
		return 1
	case w <= TightWindow:
		return 2
	case w < maxWindow:
		return 3
	default:
		return 4
	}
}

// analyze 一次扫描正文，同时产出词元集合与查询词元的覆盖窗口。
// 精排和去冗余共用结果，同一段正文只被分词一次。
func analyze(text string, terms []string) (map[string]bool, int, int) {
	set := make(map[string]bool)
	want := make(map[string]bool, len(terms))
	for _, t := range terms {
		want[t] = true
	}
	var pts []coverPoint
	hit := make(map[string]bool)
	for _, tk := range Tokenize(text) {
		if Informative(tk.Text) {
			set[tk.Text] = true
		}
		if want[tk.Text] {
			pts = append(pts, coverPoint{pos: tk.Pos, term: tk.Text})
			hit[tk.Text] = true
		}
	}
	return set, coverWindow(pts, len(hit)), len(hit)
}

func termSet(text string) map[string]bool {
	set := make(map[string]bool)
	for _, t := range Tokenize(text) {
		if Informative(t.Text) {
			set[t.Text] = true
		}
	}
	return set
}

// jaccard 的分母恒为并集大小；空集合返回 0，避免除零。
func jaccard(a, b map[string]bool) float64 {
	union := len(a) + len(b)
	if union == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	return float64(inter) / float64(union-inter)
}
