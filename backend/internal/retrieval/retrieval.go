// Package retrieval 提供 RAG 链路的本地部分：结构切片、中文词元化、
// 进程内 BM25 倒排索引、倒数排名融合与本地二阶段精排。
//
// 全部是纯本地算法，不调用任何模型服务；语义召回由 qa 层经 qdrant 完成。
// 词元化只决定「拿什么去检索」和候选排序，不作为「有依据」的判定前提。
package retrieval

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Chunk 是一个结构切片。
//
// Text 永远是子块的完整正文原文，用于引用与回答（R7.5）；
// HeadingPath 是「材料标题 > 章节 > 小节」的结构路径；
// EmbedText 是送进 embedding 接口的文本（上下文前缀 + 正文），前缀不进入 Text。
type Chunk struct {
	Seq         int
	Text        string
	HeadingPath string
	EmbedText   string
}

type heading struct {
	level int
	text  string
}

// SplitDocument 按标题层级、段落与句子边界把材料切成语义子块。
//
// 与固定字符窗口不同：标题只更新结构路径、不混进正文，使「酶」这类标题词
// 通过 HeadingPath 与 EmbedText 进入检索，而引用仍是纯原文。
// 相邻块保留 overlap 个字符的尾部重叠，避免定义句被腰斩；超长段落按句末边界再切。
func SplitDocument(title, content string, size, overlap int) []Chunk {
	if size <= 0 {
		size = 300
	}
	if overlap < 0 || overlap >= size {
		overlap = size / 6
	}
	text := strings.ReplaceAll(content, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var stack []heading
	var buf []string
	var out []Chunk

	flush := func() {
		body := strings.TrimSpace(strings.Join(buf, "\n"))
		buf = nil
		if body == "" {
			return
		}
		path := headingPathOf(title, stack)
		for _, piece := range splitBody(body, size, overlap) {
			out = append(out, Chunk{
				Seq:         len(out),
				Text:        piece,
				HeadingPath: path,
				EmbedText:   path + "\n" + piece,
			})
		}
	}

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		if h, ok := parseHeading(line); ok {
			flush()
			for len(stack) > 0 && stack[len(stack)-1].level >= h.level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, h)
			continue
		}
		if len(buf) > 0 && runeLen(strings.Join(buf, "\n"))+runeLen(line)+1 > size {
			flush()
		}
		buf = append(buf, line)
	}
	flush()
	return out
}

func headingPathOf(title string, stack []heading) string {
	title = strings.TrimSpace(title)
	parts := make([]string, 0, len(stack)+1)
	if title != "" {
		parts = append(parts, title)
	}
	for _, h := range stack {
		if h.text != "" {
			parts = append(parts, h.text)
		}
	}
	if len(parts) == 0 {
		return "材料"
	}
	return strings.Join(parts, " > ")
}

// parseHeading 识别 Markdown 标题与常见中文/编号小节标题。
func parseHeading(line string) (heading, bool) {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n > 0 && n <= 6 && n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		return heading{level: n, text: strings.TrimSpace(line[n:])}, true
	}
	if utf8.RuneCountInString(line) > 60 || isSentenceEnd([]rune(line)[len([]rune(line))-1]) {
		return heading{}, false
	}
	if l, txt, ok := numberedHeading(line); ok {
		return heading{level: l, text: txt}, true
	}
	return heading{}, false
}

func numberedHeading(line string) (int, string, bool) {
	r := []rune(line)
	if len(r) >= 3 && r[0] == '第' {
		for i := 1; i < len(r); i++ {
			if r[i] == '章' || r[i] == '节' || r[i] == '讲' || r[i] == '篇' {
				return 1, strings.TrimSpace(string(r[i+1:])), true
			}
			if !isCJKNumeral(r[i]) && !unicode.IsDigit(r[i]) {
				break
			}
		}
	}
	if len(r) >= 2 && isCJKNumeral(r[0]) {
		i := 0
		for i < len(r) && isCJKNumeral(r[i]) {
			i++
		}
		if i < len(r) && (r[i] == '、' || r[i] == '．' || r[i] == '.') {
			return 2, strings.TrimSpace(string(r[i+1:])), true
		}
	}
	// 「1.1 化学本质」这类分级编号；纯「37 摄氏度」不会命中。
	i := 0
	dots := 0
	for i < len(r) && unicode.IsDigit(r[i]) {
		i++
	}
	if i == 0 || i >= len(r) {
		return 0, "", false
	}
	for i < len(r) && r[i] == '.' && i+1 < len(r) && unicode.IsDigit(r[i+1]) {
		dots++
		i += 2
		for i < len(r) && unicode.IsDigit(r[i]) {
			i++
		}
	}
	if dots == 0 {
		return 0, "", false
	}
	if i < len(r) && (r[i] == ' ' || r[i] == '\t' || r[i] == '、' || r[i] == '．') {
		return dots + 1, strings.TrimSpace(string(r[i+1:])), true
	}
	return 0, "", false
}

func isCJKNumeral(r rune) bool {
	return strings.ContainsRune("一二三四五六七八九十百零", r)
}

func splitBody(body string, size, overlap int) []string {
	r := []rune(body)
	if len(r) <= size {
		return []string{body}
	}
	var out []string
	start := 0
	for start < len(r) {
		end := start + size
		if end >= len(r) {
			end = len(r)
		} else {
			cut := -1
			for i := end; i > start+size/2; i-- {
				if isSentenceEnd(r[i-1]) {
					cut = i
					break
				}
			}
			if cut > start {
				end = cut
			}
		}
		if piece := strings.TrimSpace(string(r[start:end])); piece != "" {
			out = append(out, piece)
		}
		if end == len(r) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return out
}

func isSentenceEnd(r rune) bool {
	return strings.ContainsRune("。！？；!?;.\n", r)
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// --- 词元化 ---

// Term 是一个可检索词元；Pos 是它在原文中的 rune 下标，用于覆盖窗口计算。
type Term struct {
	Text string
	Pos  int
}

// Tokenize 生成中文单字、相邻中文 bigram 与 ASCII 词元。
// 中文单字不可省：教学材料里「酶」「肽」这类单字概念必须能被词面召回。
func Tokenize(s string) []Term {
	r := []rune(s)
	var out []Term
	var word []rune
	wordStart := 0
	var cjk []rune
	cjkStart := -1

	flushWord := func() {
		if len(word) > 0 {
			out = append(out, Term{Text: strings.ToLower(string(word)), Pos: wordStart})
			word = nil
		}
	}
	flushCJK := func() {
		for i, c := range cjk {
			p := cjkStart + i
			out = append(out, Term{Text: string(c), Pos: p})
			if i+1 < len(cjk) {
				out = append(out, Term{Text: string(cjk[i : i+2]), Pos: p})
			}
		}
		cjk = nil
		cjkStart = -1
	}

	for i, c := range r {
		if unicode.Is(unicode.Han, c) {
			flushWord()
			if cjkStart < 0 {
				cjkStart = i
			}
			cjk = append(cjk, c)
			continue
		}
		flushCJK()
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			if len(word) == 0 {
				wordStart = i
			}
			word = append(word, c)
		} else {
			flushWord()
		}
	}
	flushWord()
	flushCJK()
	return out
}

// cjkStopwords 与 functionRunes 只用于剔除检索噪声（疑问词、功能字组合），
// 决定「拿什么去搜」，不再作为「有依据」的门禁。
var cjkStopwords = map[string]struct{}{
	"如何": {}, "何在": {}, "什么": {}, "怎么": {}, "怎样": {}, "为何": {},
	"哪里": {}, "哪个": {}, "哪些": {}, "多少": {}, "是否": {},
	"可以": {}, "一个": {}, "这个": {}, "那个": {}, "我们": {}, "你们": {}, "他们": {}, "它们": {},
	"因为": {}, "所以": {}, "但是": {}, "如果": {}, "然后": {}, "就是": {}, "都是": {}, "还是": {},
	"只是": {}, "而是": {}, "不是": {}, "没有": {}, "有些": {}, "这些": {}, "那些": {},
	"以及": {}, "关于": {}, "对于": {}, "由于": {}, "根据": {}, "按照": {}, "通过": {}, "进行": {},
	"时候": {}, "问题": {}, "材料": {}, "资料": {}, "内容": {}, "介绍": {}, "说明": {},
	"其中": {}, "之间": {}, "之后": {}, "之前": {}, "以上": {}, "以下": {}, "这样": {}, "那样": {},
	"解释": {}, "主题": {}, "涉及": {}, "本班": {}, "相关": {}, "完全": {}, "请解释": {},
}

var asciiStopwords = map[string]struct{}{
	"the": {}, "is": {}, "are": {}, "was": {}, "were": {}, "of": {}, "and": {}, "or": {},
	"to": {}, "in": {}, "on": {}, "at": {}, "for": {}, "with": {}, "what": {}, "which": {},
	"who": {}, "whom": {}, "whose": {}, "how": {}, "why": {}, "where": {}, "when": {},
	"can": {}, "could": {}, "should": {}, "would": {}, "do": {}, "does": {}, "did": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "you": {}, "we": {}, "they": {},
	"my": {}, "your": {}, "our": {}, "their": {}, "be": {}, "been": {}, "being": {},
	"as": {}, "by": {}, "from": {}, "not": {}, "no": {}, "yes": {},
}

const functionRunes = "的了是在和与或及吗呢吧啊呀哦么什哪其之这那有请为把被从对给"
const fillerRunes = "如何怎谁几多少都里个上下中前后内外等每各某该此但也还又更最很太"

// Informative 判断词元是否携带主题信息，可用于检索。
func Informative(t string) bool {
	if t == "" {
		return false
	}
	if _, bad := cjkStopwords[t]; bad {
		return false
	}
	r := []rune(t)
	if unicode.Is(unicode.Han, r[0]) {
		for _, c := range r {
			if strings.ContainsRune(functionRunes, c) {
				return false
			}
		}
		return true
	}
	if len(t) < 2 {
		return false
	}
	if _, bad := asciiStopwords[t]; bad {
		return false
	}
	return true
}

// QueryTerms 抽出用于检索的去重词元，保序。
//
// 分层：优先 bigram 与英文词（区分度高、posting 短）；一个都没有时回退到
// 中文单字，让「酶是什么」这类单字概念仍有确定入口。回退时滤掉疑问虚字，
// 避免纯泛词把材料里的常用字当成依据。
func QueryTerms(q string) []string {
	seen := make(map[string]bool)
	var multi, single []string
	for _, t := range Tokenize(q) {
		if !Informative(t.Text) || seen[t.Text] {
			continue
		}
		seen[t.Text] = true
		if runeLen(t.Text) >= 2 {
			multi = append(multi, t.Text)
			continue
		}
		r := []rune(t.Text)
		if unicode.Is(unicode.Han, r[0]) && strings.ContainsRune(fillerRunes, r[0]) {
			continue
		}
		single = append(single, t.Text)
	}
	if len(multi) > 0 {
		return multi
	}
	return single
}

// QueryVariants 生成本次批量 embedding 的输入。
//
// 短概念问句（如「酶是什么」）额外送一个纯概念短语，让语义路有一个
// 不含疑问词噪声的查询向量；长问题保持原句单路。全部变体由一次批量请求完成。
func QueryVariants(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	out := []string{q}
	if utf8.RuneCountInString(q) <= 14 {
		terms := QueryTerms(q)
		if len(terms) > 0 {
			concept := strings.Join(terms, " ")
			if concept != "" && concept != q {
				out = append(out, concept)
			}
		}
	}
	return out
}

// MinCoverWindow 求覆盖片段中全部已命中词元的最小窗口（rune 数）。
// 返回 (窗口宽度, 命中的不同词元数)；无命中时窗口为 maxWindow。
// 「词集中在同一句」→ 窗口小；「散落各处」→ 窗口大，用于二阶段精排与证据判定。
const maxWindow = 1 << 30

// coverPoint 是一个已命中词元在正文中的位置。
type coverPoint struct {
	pos  int
	term string
}

// coverWindow 用滑动窗口求覆盖全部已命中词元的最小宽度（rune）。
// 抽成独立函数，让「词元集合」和「覆盖窗口」能共用同一次全文分词。
func coverWindow(pts []coverPoint, need int) int {
	if need <= 0 || len(pts) == 0 {
		return maxWindow
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].pos < pts[j].pos })
	count := make(map[string]int)
	distinct, left, best := 0, 0, maxWindow
	for right := range pts {
		if count[pts[right].term] == 0 {
			distinct++
		}
		count[pts[right].term]++
		for distinct == need {
			width := pts[right].pos - pts[left].pos + runeLen(pts[right].term)
			if width < best {
				best = width
			}
			count[pts[left].term]--
			if count[pts[left].term] == 0 {
				distinct--
			}
			left++
		}
	}
	return best
}

func MinCoverWindow(text string, terms []string) (int, int) {
	if len(terms) == 0 {
		return maxWindow, 0
	}
	want := make(map[string]bool, len(terms))
	for _, t := range terms {
		want[t] = true
	}
	var pts []coverPoint
	hit := make(map[string]bool)
	for _, tk := range Tokenize(text) {
		if want[tk.Text] {
			pts = append(pts, coverPoint{pos: tk.Pos, term: tk.Text})
			hit[tk.Text] = true
		}
	}
	return coverWindow(pts, len(hit)), len(hit)
}

// Coverage 返回命中词元占查询词元的比例。
func Coverage(hits, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(hits) / float64(total)
}
