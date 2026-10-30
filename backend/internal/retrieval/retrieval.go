// Package retrieval 负责文本切片、关键词打分与向量检索（design D4）。
package retrieval

import (
	"math"
	"strings"
	"unicode"
)

// Split 将文本切成带重叠的片段。
//
// 策略：优先在段落 / 句末换行处断开，避免把一句话劈成两半；
// 单段超长时按字符窗口切分，窗口之间保留 overlap 个字符的重叠以维持上下文。
func Split(text string, size, overlap int) []string {
	if size <= 0 {
		size = 500
	}
	if overlap < 0 || overlap >= size {
		overlap = size / 6
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var chunks []string
	var buf []rune

	flush := func() {
		s := strings.TrimSpace(string(buf))
		if s != "" {
			chunks = append(chunks, s)
		}
	}

	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		pr := []rune(para)
		// 单段超过窗口：按字符窗口切，段内直接切分。
		if len(pr) > size {
			flush()
			buf = buf[:0]
			for start := 0; start < len(pr); start += size - overlap {
				end := start + size
				if end > len(pr) {
					end = len(pr)
				}
				if c := strings.TrimSpace(string(pr[start:end])); c != "" {
					chunks = append(chunks, c)
				}
				if end == len(pr) {
					break
				}
			}
			continue
		}
		// 常规段落：累积到接近窗口再落片。
		if len(buf)+len(pr)+1 > size && len(buf) > 0 {
			flush()
			// 用上一片尾部作为重叠上下文。
			buf = tail(buf, overlap)
		}
		if len(buf) > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, pr...)
	}
	flush()
	return chunks
}

func tail(r []rune, n int) []rune {
	if len(r) <= n {
		return append([]rune(nil), r...)
	}
	return append([]rune(nil), r[len(r)-n:]...)
}

// --- 关键词打分（BM25 近似）---

// MinEvidenceHits 是一条片段成为「依据」所需的最少内容词命中数。
//
// 只命中一个强词（或只命中泛词）不足以证明材料讲了这件事：
// 中文相邻 bigram 会把「如何 / 都是 / 式的」也算成强词，单命中门槛会被这些词击穿（R5.4）。
const MinEvidenceHits = 2

// Match 描述一条文档与查询的词面匹配结果。
type Match struct {
	// Score 是 BM25 近似分，仅用于排序。
	Score float64
	// Coverage 是命中的内容词占查询全部内容词的比例。
	Coverage float64
	// ContentHits 是命中的唯一内容词个数，用于判定「是否存在词面证据」。
	ContentHits int
}

// term 是一个词元；strong 表示「有信息量」（中文 bigram 或长度≥2 的 ASCII 词），
// content 表示该词携带主题信息（排除停用词与功能字组合）。
type term struct {
	text    string
	strong  bool
	content bool
}

// cjkStopwords 是高频泛词 / 疑问词 / 连接词，不携带学科信息。
//
// 这些词在多份材料里都会出现，若计入覆盖率，任何问题都能"看起来有依据"（R5.4）。
var cjkStopwords = map[string]struct{}{
	"如何": {}, "何在": {}, "什么": {}, "怎么": {}, "怎样": {}, "为何": {},
	"哪里": {}, "哪个": {}, "哪些": {}, "多少": {}, "是否": {},
	"可以": {}, "一个": {}, "这个": {}, "那个": {}, "我们": {}, "你们": {}, "他们": {}, "它们": {},
	"因为": {}, "所以": {}, "但是": {}, "如果": {}, "然后": {}, "就是": {}, "都是": {}, "还是": {},
	"只是": {}, "而是": {}, "不是": {}, "没有": {}, "有些": {}, "这些": {}, "那些": {},
	"以及": {}, "关于": {}, "对于": {}, "由于": {}, "根据": {}, "按照": {}, "通过": {}, "进行": {},
	"时候": {}, "问题": {}, "材料": {}, "资料": {}, "内容": {}, "介绍": {}, "说明": {},
	"其中": {}, "之间": {}, "之后": {}, "之前": {}, "以上": {}, "以下": {}, "这样": {}, "那样": {},
}

// asciiStopwords 是英文常用功能词；英文主题词仍按长度≥2 视为内容词。
var asciiStopwords = map[string]struct{}{
	"the": {}, "is": {}, "are": {}, "was": {}, "were": {}, "of": {}, "and": {}, "or": {},
	"to": {}, "in": {}, "on": {}, "at": {}, "for": {}, "with": {}, "what": {}, "which": {},
	"who": {}, "whom": {}, "whose": {}, "how": {}, "why": {}, "where": {}, "when": {},
	"can": {}, "could": {}, "should": {}, "would": {}, "do": {}, "does": {}, "did": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "you": {}, "we": {}, "they": {},
	"my": {}, "your": {}, "our": {}, "their": {}, "be": {}, "been": {}, "being": {},
	"as": {}, "by": {}, "from": {}, "not": {}, "no": {}, "yes": {},
}

// functionRunes 是中文功能字；bigram 只要含其中之一，就不算内容词。
// 这样可以自动排除「式的 / 的量 / 国的 / 的首 / 程是」这类跨词边界的噪声。
const functionRunes = "的了是在和与或及吗呢吧啊呀哦么什哪"

// ScoreKeywords 对文档集按查询词做 BM25 近似打分。
//
// 返回与 docs 等长的 Match 列表，以及查询中唯一内容词的总数。
// 调用方应以「ContentHits >= MinEvidenceHits」作为词面证据的硬前提，
// 再用 Coverage（或向量相似度）决定是否足够相关（R5.4）。
func ScoreKeywords(query string, docs []string) ([]Match, int) {
	terms := tokenize(query)
	out := make([]Match, len(docs))
	if len(terms) == 0 {
		return out, 0
	}

	contentTotal := 0
	uniq := map[string]bool{}
	for _, t := range terms {
		if t.content && !uniq[t.text] {
			uniq[t.text] = true
			contentTotal++
		}
	}

	// 文档频率（用于 IDF）。
	df := map[string]int{}
	docTokens := make([]map[string]int, len(docs))
	for i, d := range docs {
		tf := map[string]int{}
		for _, t := range tokenize(d) {
			tf[t.text]++
		}
		docTokens[i] = tf
		for t := range tf {
			df[t]++
		}
	}

	n := float64(len(docs))
	avg := 0.0
	for _, tf := range docTokens {
		avg += float64(len(tf))
	}
	if n > 0 {
		avg /= n
	}
	const (
		k1 = 1.2
		b  = 0.75
	)
	for i, tf := range docTokens {
		dl := float64(len(tf))
		matchedContent := map[string]bool{}
		for _, t := range terms {
			f := float64(tf[t.text])
			if f == 0 {
				continue
			}
			if t.content {
				matchedContent[t.text] = true
			}
			idf := logIDF(n, float64(df[t.text]))
			denom := f + k1*(1-b+b*dl/maxf(avg, 1))
			out[i].Score += idf * f * (k1 + 1) / denom
		}
		out[i].ContentHits = len(matchedContent)
		if contentTotal > 0 {
			out[i].Coverage = float64(len(matchedContent)) / float64(contentTotal)
		}
	}
	return out, contentTotal
}

// tokenize 生成词元：ASCII 词（长度≥2 记 strong）+ 中文单字 + 相邻中文 bigram（strong）。
// 只有内容词（非停用词、非功能字组合）参与「有无依据」的判定。
func tokenize(s string) []term {
	var out []term
	var ascii []rune
	var cjk []rune

	flushAscii := func() {
		if len(ascii) > 0 {
			w := strings.ToLower(string(ascii))
			strong := len(ascii) >= 2
			_, stop := asciiStopwords[w]
			out = append(out, term{text: w, strong: strong, content: strong && !stop})
			ascii = ascii[:0]
		}
	}
	flushCJK := func() {
		for i, r := range cjk {
			out = append(out, term{text: string(r)})
			if i+1 < len(cjk) {
				bigram := string(cjk[i : i+2])
				out = append(out, term{text: bigram, strong: true, content: isContentBigram(bigram)})
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			flushAscii()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			ascii = append(ascii, r)
		default:
			flushAscii()
			flushCJK()
		}
	}
	flushAscii()
	flushCJK()
	return out
}

// isContentBigram 判断一个中文 bigram 是否携带主题信息。
func isContentBigram(s string) bool {
	if _, stop := cjkStopwords[s]; stop {
		return false
	}
	for _, r := range s {
		if strings.ContainsRune(functionRunes, r) {
			return false
		}
	}
	return true
}

func logIDF(n, df float64) float64 {
	if df <= 0 {
		return 0
	}
	return 1 + math.Log2(1+(n-df+0.5)/(df+0.5))
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
