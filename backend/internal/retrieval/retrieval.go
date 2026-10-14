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

// term 是一个词元；strong 表示「有信息量」（中文 bigram 或长度≥2 的 ASCII 词）。
type term struct {
	text   string
	strong bool
}

// ScoreKeywords 对文档集按查询词做 BM25 近似打分。
//
// 返回与 docs 等长的三个数组：
//   - scores：BM25 近似分（用于排序）
//   - coverage：命中的「有信息量查询词」占比（用于判定相关性）
//   - strongHits：命中的有信息量词个数（用于判定「是否存在词面证据」）
//
// coverage / strongHits 共同支撑「材料里到底有没有相关内容」的判定，
// 避免仅命中「的 / 材料」这类常见字、或纯靠最近邻就当作有依据（spec R5.2）。
func ScoreKeywords(query string, docs []string) (scores, coverage []float64, strongHits []int) {
	terms := tokenize(query)
	scores = make([]float64, len(docs))
	coverage = make([]float64, len(docs))
	strongHits = make([]int, len(docs))
	if len(terms) == 0 {
		return scores, coverage, strongHits
	}

	strongTotal := 0
	uniq := map[string]bool{}
	for _, t := range terms {
		if t.strong && !uniq[t.text] {
			uniq[t.text] = true
			strongTotal++
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
		matchedStrong := map[string]bool{}
		for _, t := range terms {
			f := float64(tf[t.text])
			if f == 0 {
				continue
			}
			if t.strong {
				matchedStrong[t.text] = true
			}
			idf := logIDF(n, float64(df[t.text]))
			denom := f + k1*(1-b+b*dl/maxf(avg, 1))
			scores[i] += idf * f * (k1 + 1) / denom
		}
		strongHits[i] = len(matchedStrong)
		if strongTotal > 0 {
			coverage[i] = float64(len(matchedStrong)) / float64(strongTotal)
		}
	}
	return scores, coverage, strongHits
}

// tokenize 生成词元：ASCII 词（长度≥2 记 strong）+ 中文单字 + 相邻中文 bigram（strong）。
func tokenize(s string) []term {
	var out []term
	var ascii []rune
	var cjk []rune

	flushAscii := func() {
		if len(ascii) > 0 {
			w := strings.ToLower(string(ascii))
			out = append(out, term{text: w, strong: len(ascii) >= 2})
			ascii = ascii[:0]
		}
	}
	flushCJK := func() {
		for i, r := range cjk {
			out = append(out, term{text: string(r)})
			if i+1 < len(cjk) {
				out = append(out, term{text: string(cjk[i : i+2]), strong: true})
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
