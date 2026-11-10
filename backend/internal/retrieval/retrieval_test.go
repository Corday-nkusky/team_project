package retrieval

import (
	"strings"
	"testing"
)

// --- 结构切片 ---

func TestSplitDocumentKeepsHeadingOutOfSnippet(t *testing.T) {
	content := "# 酶\n\n## 2 化学本质\n\n它由蛋白质构成，具有高度专一性与高效性。\n" +
		"少数 RNA 也具有催化活性。\n\n## 3 影响因素\n\n温度与 pH 会改变酶活性。"
	parts := SplitDocument("生物化学讲义", content, 300, 40)
	if len(parts) == 0 {
		t.Fatal("结构切片结果为空")
	}
	found := false
	for _, p := range parts {
		if strings.Contains(p.Text, "它由蛋白质构成") {
			found = true
			if !strings.Contains(p.HeadingPath, "酶") || !strings.Contains(p.HeadingPath, "2 化学本质") {
				t.Fatalf("定义句子块的章节路径应含标题链，实际 %q", p.HeadingPath)
			}
			if strings.Contains(p.Text, "生物化学讲义") || strings.Contains(p.Text, " > ") {
				t.Fatalf("引用原文不得混入标题前缀/路径：%q", p.Text)
			}
			if !strings.HasPrefix(p.EmbedText, p.HeadingPath) {
				t.Fatalf("嵌入文本应以上下文路径开头：%q", p.EmbedText)
			}
			if !strings.Contains(p.EmbedText, "它由蛋白质构成") {
				t.Fatalf("嵌入文本应含正文：%q", p.EmbedText)
			}
		}
	}
	if !found {
		t.Fatal("定义句未出现在任何子块中")
	}
	for i, p := range parts {
		if p.Seq != i {
			t.Fatalf("Seq 应连续：第 %d 块为 %d", i, p.Seq)
		}
	}
}

func TestSplitDocumentSplitsLongParagraphAtSentenceBoundary(t *testing.T) {
	body := strings.Repeat("酶催化反应速率随底物浓度上升而趋于饱和。", 20)
	parts := SplitDocument("讲义", body, 120, 20)
	if len(parts) < 3 {
		t.Fatalf("超长段落应按句子边界切分，实际 %d 块", len(parts))
	}
	for _, p := range parts {
		if len([]rune(p.Text)) > 140 {
			t.Fatalf("子块超过窗口上限：%d 字", len([]rune(p.Text)))
		}
		if strings.Contains(p.Text, "生物化学讲义") {
			t.Fatal("上下文前缀不得混入正文")
		}
	}
}

// --- 查询词元 ---

func TestQueryTermsShortConceptStillSearchable(t *testing.T) {
	terms := QueryTerms("酶是什么？")
	if len(terms) == 0 || !containsStr(terms, "酶") {
		t.Fatalf("单字概念必须可检索（旧逻辑的死穴），实际 %v", terms)
	}
}

func TestQueryTermsRejectsPureFiller(t *testing.T) {
	for _, q := range []string{"如何 什么 都是 哪里", "请解释一下这个那个"} {
		if got := QueryTerms(q); len(got) > 0 && !containsStr(got, "解释") {
			// 「解释」等通用词允许存在，但不得只剩疑问虚字
			if allFiller(got) {
				t.Fatalf("纯泛词查询不应产出可检索词元：%q → %v", q, got)
			}
		}
	}
	if got := QueryTerms("如何 什么 都是 哪里"); len(got) != 0 {
		t.Fatalf("纯疑问词查询词元应为空，实际 %v", got)
	}
}

func TestQueryVariantsOneBatchOnly(t *testing.T) {
	short := QueryVariants("酶是什么")
	if len(short) != 2 || short[0] != "酶是什么" {
		t.Fatalf("短概念问句应有原句+概念短语两路，实际 %v", short)
	}
	long := QueryVariants("请详细说明本班讲义中量子纠缠实验的构造与验证方法是什么")
	if len(long) != 1 {
		t.Fatalf("长问题应保持单路，实际 %v", long)
	}
}

// --- 倒排索引 ---

func testDocs() []Doc {
	return []Doc{
		{ID: 1, MaterialID: 10, Seq: 0, Text: "酶是由活细胞产生的具有催化作用的有机物，绝大多数酶的化学本质是蛋白质。", HeadingPath: "生物化学讲义 > 酶 > 2 化学本质", MaterialTitle: "生物化学讲义"},
		{ID: 2, MaterialID: 10, Seq: 1, Text: "温度与 pH 会改变酶活性，高温会使酶变性失活。", HeadingPath: "生物化学讲义 > 酶 > 3 影响因素", MaterialTitle: "生物化学讲义"},
		{ID: 3, MaterialID: 11, Seq: 0, Text: "光合作用的光反应发生在叶绿体类囊体薄膜上。", HeadingPath: "植物生理 > 光合作用 > 1 光反应", MaterialTitle: "植物生理"},
		{ID: 4, MaterialID: 11, Seq: 1, Text: "暗反应在叶绿体基质中固定二氧化碳。", HeadingPath: "植物生理 > 光合作用 > 2 暗反应", MaterialTitle: "植物生理"},
	}
}

func TestInvertedIndexSearchOnlyVisitsMatchedPostings(t *testing.T) {
	ix := NewIndex()
	ix.Build(testDocs())
	if ix.Len() != 4 {
		t.Fatalf("索引应有 4 条切片，实际 %d", ix.Len())
	}
	terms := QueryTerms("酶的化学本质是什么")
	hits := ix.Search(terms, 10)
	if len(hits) == 0 || hits[0] != 1 {
		t.Fatalf("BM25 应把讲化学本质的切片排第一，实际 %v", hits)
	}
	if got := ix.Search([]string{"完全不存在的词元组合XYZ"}, 10); len(got) != 0 {
		t.Fatalf("未命中词元不应返回候选，实际 %v", got)
	}
}

func TestHeadingRouteRescuesTitleOnlyQuery(t *testing.T) {
	ix := NewIndex()
	ix.Build(testDocs())
	terms := QueryTerms("酶是什么")
	hits := ix.SearchHeading(terms, 10)
	if len(hits) == 0 {
		t.Fatalf("标题路径命中应能召回「酶是什么」这类短问句")
	}
	for _, id := range hits {
		d, _ := ix.Doc(id)
		if !strings.Contains(d.HeadingPath, "酶") {
			t.Fatalf("标题路返回了路径不含查询词的切片 %d: %q", id, d.HeadingPath)
		}
	}
}

func TestIndexNeighboursStayWithinMaterial(t *testing.T) {
	ix := NewIndex()
	ix.Build(testDocs())
	ids := ix.Neighbours(10, 0)
	if len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("相邻片应只取同材料 seq±1，实际 %v", ids)
	}
}

// --- RRF 与精排 ---

func TestFuseRRFUsesRanksNotRawScores(t *testing.T) {
	// 两路名次互换时 7/8 的 RRF 分数严格相等，按 ID 升序确定性并列；
	// 两者都必须排在只在末尾出现过的 9 之前。
	got := FuseRRF([][]int64{{7, 8, 9}, {8, 7, 9}}, 3)
	if len(got) != 3 || got[0] != 7 || got[1] != 8 || got[2] != 9 {
		t.Fatalf("融合应按名次加权并确定性并列，实际 %v", got)
	}
	got2 := FuseRRF([][]int64{{7, 8, 9}, {8, 7, 9}}, 3)
	for i := range got {
		if got[i] != got2[i] {
			t.Fatalf("融合结果必须可复现：%v vs %v", got, got2)
		}
	}
	// 单路独占第一名、另一路靠后的 ID 应被两路都进前二的对象压过。
	got3 := FuseRRF([][]int64{{5, 1, 2}, {1, 2, 5}}, 3)
	if got3[0] != 1 && got3[0] != 5 {
		t.Fatalf("融合首位应来自两路都靠前的对象，实际 %v", got3)
	}
}

func TestMinCoverWindowDistinguishesConcentratedFromScattered(t *testing.T) {
	terms := QueryTerms("光合作用 场所")
	tight, hitsT := MinCoverWindow("光合作用的场所是叶绿体。", terms)
	loose, _ := MinCoverWindow("光合作用分两阶段。"+strings.Repeat("中间内容", 100)+"场所另见附录。", terms)
	if hitsT == 0 || tight >= loose {
		t.Fatalf("集中命中窗口应明显小于散落命中：tight=%d loose=%d hits=%d", tight, loose, hitsT)
	}
}

func TestRerankAdmitsShortConceptAndRejectsCrossDomain(t *testing.T) {
	ix := NewIndex()
	ix.Build(testDocs())

	admit := []Candidate{{ID: 1, Doc: mustDoc(ix, 1), VecScore: 0.68, RRFRank: 1}}
	got := Rerank(admit, QueryTerms("酶是什么"), EvidencePolicy{
		MinCoverage: 0.30, MinVecScore: 0.55, VecEnabled: true, QueryVecScore: 0.68,
	})
	if len(got) != 1 || !got[0].Evidence {
		t.Fatalf("短概念问句应判为有依据：%+v", got)
	}

	// R5.4 历史误判：泛词 / 跨域问题，即使召回返回了最近邻也不得放行。
	reject := []string{
		"贝尔不等式的量子力学推导过程是什么？",
		"如何在家做红烧肉？",
		"法国的首都是哪里？",
	}
	for _, q := range reject {
		cands := []Candidate{
			{ID: 3, Doc: mustDoc(ix, 3), VecScore: 0.41, RRFRank: 1},
			{ID: 1, Doc: mustDoc(ix, 1), VecScore: 0.38, RRFRank: 2},
		}
		r := Rerank(cands, QueryTerms(q), EvidencePolicy{
			MinCoverage: 0.30, MinVecScore: 0.55, VecEnabled: true, QueryVecScore: 0.41,
		})
		for _, c := range r {
			if c.Evidence {
				t.Fatalf("「%s」不应构成依据（片段 %d 通过）", q, c.ID)
			}
		}
	}
}

func TestRerankZeroKeyStillAnswersDoubleCharConcept(t *testing.T) {
	ix := NewIndex()
	ix.Build(testDocs())
	cands := []Candidate{{ID: 1, Doc: mustDoc(ix, 1), RRFRank: 1}}
	got := Rerank(cands, QueryTerms("酶的化学本质"), EvidencePolicy{
		MinCoverage: 0.30, MinVecScore: 0.55, VecEnabled: false,
	})
	if len(got) != 1 || !got[0].Evidence {
		t.Fatalf("零密钥降级下双字概念问题应能作答：%+v", got)
	}
}

func TestDeduplicateDropsOverlappingChunks(t *testing.T) {
	base := mustDoc(NewIndexWithDocs(), 1)
	dup := base
	dup.ID = 99
	dup.Text = base.Text + "（补充一句相同的说明）"
	ordered := []Candidate{
		{ID: 1, Doc: base, Evidence: true},
		{ID: 99, Doc: dup, Evidence: true},
		{ID: 3, Doc: Doc{ID: 3, MaterialID: 10, Text: "光合作用发生在叶绿体。"}, Evidence: true},
	}
	got := Deduplicate(ordered, 3, 0.75)
	if len(got) != 2 || got[1].ID != 3 {
		t.Fatalf("高度重叠片段应被剔除，top-k 覆盖不同方面，实际 %v", idsOf(got))
	}
}

// --- 辅助 ---

func NewIndexWithDocs() *Index {
	ix := NewIndex()
	ix.Build(testDocs())
	return ix
}

func mustDoc(ix *Index, id int64) Doc {
	d, ok := ix.Doc(id)
	if !ok {
		panic("missing doc")
	}
	return d
}

func idsOf(c []Candidate) []int64 {
	out := make([]int64, len(c))
	for i := range c {
		out[i] = c[i].ID
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func allFiller(list []string) bool {
	for _, v := range list {
		if !strings.ContainsRune(fillerRunes, []rune(v)[0]) {
			return false
		}
	}
	return true
}
