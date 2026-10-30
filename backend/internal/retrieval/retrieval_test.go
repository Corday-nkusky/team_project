package retrieval

import "testing"

// 本组用例来自独立走查的真实误判（docs/independent-test-report.md N01）：
// 这些查询只命中「式的 / 过程 / 如何 / 都是」等泛词，旧逻辑会把覆盖率推过 0.25，
// 从而把完全无关的材料当成依据。新判定必须把这些泛词排除在内容词之外。
func TestScoreKeywordsRejectsFillerOnlyMatches(t *testing.T) {
	cases := []struct {
		name  string
		query string
		doc   string
	}{
		{
			name:  "贝尔不等式命中式的与过程",
			query: "贝尔不等式的量子力学推导过程是什么？",
			doc:   "动物行为的研究方式的变化过程是什么？中国的学者如何观察？这些都是基础。",
		},
		{
			name:  "红烧肉命中如何与何在",
			query: "如何在家做红烧肉？",
			doc:   "这节课讨论如何理解细胞结构，以及这部分内容何在教材中出现。",
		},
		{
			name:  "法国首都命中国的与都是",
			query: "法国的首都是哪里？",
			doc:   "中国的首都都是重要城市，本节讨论城市的形成过程。",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches, total := ScoreKeywords(tc.query, []string{tc.doc})
			if total == 0 {
				t.Fatalf("查询 %q 的内容词总数不应为 0（用于验证泛词被排除，而不是没有词）", tc.query)
			}
			if matches[0].ContentHits >= MinEvidenceHits {
				t.Fatalf("泛词命中了 %d 个内容词（门槛 %d），仍会误判为有依据；coverage=%v",
					matches[0].ContentHits, MinEvidenceHits, matches[0].Coverage)
			}
		})
	}
}

// 真实误判的长查询：材料里「诺贝尔」含「贝尔」、「I-量子光合作用」含「量子」，
// 再加一个「过程」，可获得 3 个内容词命中；但覆盖率 3/11≈0.27，
// 低于默认门槛 0.30（MIN_KEYWORD_COVERAGE），不应被当成依据。
func TestScoreKeywordsRejectsLongCrossDomainQuery(t *testing.T) {
	doc := "诺贝尔奖得主、科学家鲁道夫·马库斯发现电子转移链的作用和意义。" +
		"奥托·海因里希·瓦尔堡和迪安·伯克发现I-量子光合作用的反应，分裂二氧化碳，由呼吸作用激活。" +
		"这个过程的关键参与者是细胞内部的叶绿体。"
	const defaultCoverage = 0.30 // 与 MIN_KEYWORD_COVERAGE 默认值保持一致

	matches, total := ScoreKeywords("贝尔不等式的量子力学推导过程是什么？", []string{doc})
	m := matches[0]
	if m.ContentHits < MinEvidenceHits {
		t.Fatalf("该语料应命中至少 %d 个偶然子串以复现历史误判，实际 %d（total=%d）",
			MinEvidenceHits, m.ContentHits, total)
	}
	if m.Coverage >= defaultCoverage {
		t.Fatalf("覆盖率 %.3f 不应达到默认门槛 %.2f（命中 %d/%d）",
			m.Coverage, defaultCoverage, m.ContentHits, total)
	}
}

func TestScoreKeywordsAcceptsRealCourseQuestion(t *testing.T) {
	cases := []struct {
		name  string
		query string
		doc   string
	}{
		{
			name:  "乳酸菌繁殖温度",
			query: "乳酸菌的最佳繁殖温度是多少？",
			doc:   "乳酸菌在 37 摄氏度时繁殖最佳，温度过高会抑制生长。",
		},
		{
			name:  "光合作用",
			query: "光合作用是什么？",
			doc:   "光合作用是植物利用光能合成有机物的过程。",
		},
		{
			name:  "水循环环节",
			query: "水循环包括哪些环节？",
			doc:   "水循环包括蒸发、凝结、降水与径流等环节。",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches, total := ScoreKeywords(tc.query, []string{tc.doc})
			m := matches[0]
			if total < MinEvidenceHits {
				t.Fatalf("正常问题的内容词总数只有 %d，查询词被过度过滤: %q", total, tc.query)
			}
			if m.ContentHits < MinEvidenceHits {
				t.Fatalf("正常问题只命中 %d 个内容词（门槛 %d）: %q", m.ContentHits, MinEvidenceHits, tc.query)
			}
			if m.Coverage < 0.25 {
				t.Fatalf("正常问题覆盖率过低: %.3f（内容词 %d/%d）", m.Coverage, m.ContentHits, total)
			}
			if m.Score <= 0 {
				t.Fatalf("命中但 BM25 分数为 0: %q", tc.query)
			}
		})
	}
}

func TestScoreKeywordsWithoutContentTerms(t *testing.T) {
	matches, total := ScoreKeywords("如何 什么 都是 哪里", []string{"如何理解什么材料，这些都是问题。"})
	if total != 0 {
		t.Fatalf("纯泛词查询的内容词总数应为 0，得到 %d", total)
	}
	for i, m := range matches {
		if m.ContentHits != 0 || m.Coverage != 0 {
			t.Fatalf("文档 %d 不应有内容词命中: %+v", i, m)
		}
	}
}

func TestIsContentBigram(t *testing.T) {
	cases := map[string]bool{
		"量子": true,
		"过程": true,
		"推导": true,
		"式的": false,
		"国的": false,
		"的首": false,
		"都是": false,
		"如何": false,
		"什么": false,
		"哪里": false,
	}
	for bigram, want := range cases {
		if got := isContentBigram(bigram); got != want {
			t.Errorf("isContentBigram(%q) = %v，期望 %v", bigram, got, want)
		}
	}
}

func TestAsciiContentTerms(t *testing.T) {
	matches, total := ScoreKeywords("what is quantum entanglement", []string{"quantum entanglement experiment"})
	m := matches[0]
	if total != 2 { // quantum + entanglement；what/is 是停用词
		t.Fatalf("英文内容词总数期望 2，得到 %d", total)
	}
	if m.ContentHits != 2 || m.Coverage != 1 {
		t.Fatalf("英文内容词命中不符合预期: %+v", m)
	}
}
