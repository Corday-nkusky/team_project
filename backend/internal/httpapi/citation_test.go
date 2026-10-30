package httpapi

import "testing"

func feed(f *citationFilter, parts ...string) string {
	out := ""
	for _, p := range parts {
		out += f.Write(p)
	}
	return out + f.Flush()
}

func TestCitationFilterDropsOutOfRange(t *testing.T) {
	f := &citationFilter{max: 1}
	got := feed(f, "材料里说的是这样[9]。 另一个结论[1]。")
	want := "材料里说的是这样。 另一个结论[1]。"
	if got != want {
		t.Fatalf("过滤结果不符：got %q want %q", got, want)
	}
	if f.Dropped() != 1 {
		t.Fatalf("应记录 1 个被丢弃的越界引用，得到 %d", f.Dropped())
	}
}

func TestCitationFilterHandlesSplitAcrossDeltas(t *testing.T) {
	f := &citationFilter{max: 2}
	var got string
	for _, p := range []string{"结论[", "9", "]，另有[", "2", "]。"} {
		got += f.Write(p)
	}
	got += f.Flush()
	if got != "结论，另有[2]。" {
		t.Fatalf("跨增量过滤结果不符：%q", got)
	}
	if f.Dropped() != 1 {
		t.Fatalf("越界引用应被丢弃一次，得到 %d", f.Dropped())
	}
}

func TestCitationFilterKeepsNonCitationBrackets(t *testing.T) {
	f := &citationFilter{max: 1}
	got := feed(f, "见图[注]，区间[1,2]，公式[3]与[1]。")
	if got != "见图[注]，区间[1,2]，公式与[1]。" {
		t.Fatalf("非引用方括号被误伤：%q", got)
	}
	if f.Dropped() != 1 {
		t.Fatalf("仅 [3] 应被丢弃，得到 %d", f.Dropped())
	}
}

func TestCitationFilterDropsAllWhenNoCitations(t *testing.T) {
	f := &citationFilter{max: 0}
	got := feed(f, "没有来源的[1]和[9]。")
	if got != "没有来源的和。" {
		t.Fatalf("无引用清单时应丢弃全部编号：%q", got)
	}
	if f.Dropped() != 2 {
		t.Fatalf("应丢弃 2 个标记，得到 %d", f.Dropped())
	}
}

func TestCitationFilterFlushesIncompleteTail(t *testing.T) {
	f := &citationFilter{max: 3}
	if got := f.Write("未完成的["); got != "未完成的" {
		t.Fatalf("不完整的标记应暂存，实际 %q", got)
	}
	if got := f.Flush(); got != "[" {
		t.Fatalf("流结束时应放出缓冲，实际 %q", got)
	}
}
