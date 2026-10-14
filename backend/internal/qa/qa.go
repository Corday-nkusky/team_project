// Package qa 编排「检索 → 依据材料生成回答」的链路（spec R5 / R7）。
//
// 检索范围恒为「发起者所在班 + 索引状态 ready 的材料」，班级由服务端从会话推导，
// 不接受客户端参数，因而跨班内容不可能进入候选集（R2.2 / R7.2）。
package qa

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Corday-nkusky/team_project/backend/internal/llm"
	"github.com/Corday-nkusky/team_project/backend/internal/qdrant"
	"github.com/Corday-nkusky/team_project/backend/internal/retrieval"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// Collection 是 Qdrant 集合名（带项目前缀，避免与同机上其他项目共用实例时撞名）。
const Collection = "team_campusclaw_chunks"

// Citation 是回答所依据的一条材料片段。
type Citation struct {
	Index         int    `json:"index"`
	MaterialID    int64  `json:"material_id"`
	MaterialTitle string `json:"material_title"`
	ChunkID       int64  `json:"chunk_id"`
	Snippet       string `json:"snippet"`
}

// Service 编排检索与生成。
type Service struct {
	store       *store.Store
	vec         *qdrant.Client
	llm         *llm.Client
	topK        int
	chunkSize   int
	overlap     int
	minCoverage float64
	minVecScore float64
}

// New 构造 Service。
func New(st *store.Store, vec *qdrant.Client, model *llm.Client, topK, chunkSize, overlap int,
	minCoverage, minVecScore float64) *Service {
	return &Service{
		store: st, vec: vec, llm: model, topK: topK, chunkSize: chunkSize, overlap: overlap,
		minCoverage: minCoverage, minVecScore: minVecScore,
	}
}

// Index 为材料建立索引：切片入 MySQL；若配置了模型网关，再生成向量入 Qdrant。
//
// 返回 error 时调用方应把材料置为 failed（R3.3）。未配置模型网关时
// 退化为「仅关键词索引」，不视为失败。
func (s *Service) Index(ctx context.Context, materialID, classID int64, content string) error {
	texts := retrieval.Split(content, s.chunkSize, s.overlap)
	if len(texts) == 0 {
		return fmt.Errorf("切片为空")
	}
	chunks, err := s.store.ReplaceChunks(ctx, materialID, classID, texts)
	if err != nil {
		return err
	}

	if !s.llm.Enabled() {
		return nil // 零密钥降级：仅关键词检索
	}

	vecs, err := s.llm.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("生成嵌入失败: %w", err)
	}
	if err := s.vec.EnsureCollection(ctx, Collection, len(vecs[0])); err != nil {
		return err
	}
	if err := s.vec.DeleteByMaterial(ctx, Collection, materialID); err != nil {
		return err
	}
	points := make([]qdrant.Point, len(chunks))
	for i, c := range chunks {
		points[i] = qdrant.Point{
			ID:     uint64(c.ID),
			Vector: vecs[i],
			Payload: map[string]any{
				"class_id":    classID,
				"material_id": materialID,
				"chunk_id":    c.ID,
			},
		}
	}
	if err := s.vec.Upsert(ctx, Collection, points); err != nil {
		return err
	}
	return nil
}

// Retrieve 返回本班范围内与问题最相关的片段（关键词 + 向量融合，design D4）。
func (s *Service) Retrieve(ctx context.Context, classID int64, question string) ([]Citation, error) {
	chunks, err := s.store.ListChunksByClass(ctx, classID)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}

	texts := make([]string, len(chunks))
	byID := make(map[int64]int, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
		byID[c.ID] = i
	}

	// 关键词路（恒可用）：分数 + 覆盖率 + 词面证据。
	kw, cov, strongHits := retrieval.ScoreKeywords(question, texts)

	// 向量路（仅在配置了模型网关时）：保留原始余弦分用于门槛判定。
	var vecRaw []float64
	if s.llm.Enabled() {
		if qv, err := s.llm.Embed(ctx, []string{question}); err == nil {
			if hits, err := s.vec.Search(ctx, Collection, qv[0], classID, s.topK*3); err == nil {
				vecRaw = make([]float64, len(chunks))
				for _, h := range hits {
					id, ok := payloadInt(h.Payload, "chunk_id")
					if !ok {
						continue
					}
					if idx, ok := byID[id]; ok {
						vecRaw[idx] = h.Score
					}
				}
			}
		}
	}

	// 相关性门槛（spec R5.2）：一条片段要成为「依据」，必须先有**词面证据**
	// （命中至少一个有信息量的查询词），再叠加「覆盖率足够」或「语义相似度足够」。
	//
	// 之所以要求词面证据：向量检索总会返回最近邻且分数非零，仅凭相似度会让
	// 任何问题都"看起来有依据"。本版不加重排（design D4），以词面证据为前置更稳妥。
	fused := make([]float64, len(chunks))
	kwN := append([]float64(nil), kw...)
	norm(kwN)
	vecN := append([]float64(nil), vecRaw...)
	norm(vecN)
	anyEvidence := false
	for i := range chunks {
		if strongHits[i] == 0 {
			continue
		}
		byKeyword := cov[i] >= s.minCoverage
		byVector := vecRaw != nil && vecRaw[i] >= s.minVecScore
		if !byKeyword && !byVector {
			continue
		}
		anyEvidence = true
		if vecRaw != nil {
			fused[i] = 0.6*kwN[i] + 0.4*vecN[i]
		} else {
			fused[i] = kwN[i]
		}
	}
	if !anyEvidence {
		return nil, nil
	}

	order := make([]int, len(chunks))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return fused[order[a]] > fused[order[b]] })

	var out []Citation
	for _, idx := range order {
		if fused[idx] <= 0 {
			break
		}
		c := chunks[idx]
		out = append(out, Citation{
			MaterialID:    c.MaterialID,
			MaterialTitle: c.MaterialTitle,
			ChunkID:       c.ID,
			Snippet:       snippet(c.Text, 300),
		})
		if len(out) >= s.topK {
			break
		}
	}
	for i := range out {
		out[i].Index = i + 1
	}
	return out, nil
}

// LLMEnabled 表示是否走模型生成（否则用抽取式降级回答）。
func (s *Service) LLMEnabled() bool { return s.llm.Enabled() }

// ChatStream 以指定系统/用户提示词发起流式生成。
func (s *Service) ChatStream(ctx context.Context, system, user string) (<-chan llm.Event, error) {
	return s.llm.ChatStream(ctx, system, user)
}

// SystemPrompt 约束模型只依据给定资料作答并以 [n] 标注出处。
func SystemPrompt() string {
	return "你是一位课程助教，只能依据下面提供的「资料」回答学生的问题。要求：\n" +
		"1. 每个结论后面都必须用 [编号] 标注它来自哪一条资料，例如「教师需要在上传页提交讲义[1]。」；\n" +
		"2. 只使用资料中出现的信息，不得补充资料之外的常识或推测；\n" +
		"3. 若资料中确实没有能回答该问题的内容，只回复这一句：资料中未找到相关内容；\n" +
		"4. 使用简体中文，回答简洁。"
}

// UserPrompt 组装带编号的资料与问题。
func UserPrompt(question string, cites []Citation) string {
	var b strings.Builder
	b.WriteString("资料：\n")
	for _, c := range cites {
		fmt.Fprintf(&b, "[%d]（来自《%s》）%s\n", c.Index, c.MaterialTitle, c.Snippet)
	}
	b.WriteString("\n问题：")
	b.WriteString(question)
	return b.String()
}

// FallbackAnswer 在未配置模型网关时，用检索片段直接拼出可溯源的回答。
//
// 这是零密钥降级路径：回答完全来自本班材料，仍带 [n] 引用（R7.1）。
func FallbackAnswer(question string, cites []Citation) string {
	var b strings.Builder
	b.WriteString("（当前未配置模型网关，以下为直接摘自本班材料的检索结果）\n\n")
	for _, c := range cites {
		fmt.Fprintf(&b, "[%d] 《%s》：%s\n\n", c.Index, c.MaterialTitle, c.Snippet)
	}
	b.WriteString("以上内容均来自你的班级已上传的材料，可点击引用查看原文。")
	return b.String()
}

const (
	// NoEvidenceText 是检索落空时的固定文案（R5.2）。
	NoEvidenceText = "资料中未找到相关内容"
)

// norm 将分数数组归一化到 [0,1]（除以最大值）。
func norm(scores []float64) {
	max := 0.0
	for _, s := range scores {
		if s > max {
			max = s
		}
	}
	if max <= 0 {
		return
	}
	for i := range scores {
		scores[i] /= max
	}
}

func payloadInt(p map[string]any, key string) (int64, bool) {
	v, ok := p[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func snippet(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
