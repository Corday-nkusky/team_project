package httpapi

import (
	"strconv"
	"strings"
)

// citationFilter 在流式输出中只保留引用清单范围内的 [n]（spec R7.4）。
//
// 模型增量可能把 "[9]" 拆成 "["、"9"、"]" 三个事件，因此必须跨事件缓冲；
// 只缓冲形如 "[数字" 的最长 4 个字符，其余文本立即放行。
// 越界编号被丢弃（不进入展示与落库），并计数供告警使用。
type citationFilter struct {
	max     int
	pending []rune
	dropped int
}

// maxCitationDigits 是合法引用编号的最大位数（topK 上限 20，3 位足够容纳异常输出）。
const maxCitationDigits = 3

// Write 接收一段增量，返回可以安全发送的文本；可能因等待 "]" 而暂存尾部。
func (f *citationFilter) Write(s string) string {
	if s == "" {
		return ""
	}
	f.pending = append(f.pending, []rune(s)...)
	return f.process(false)
}

// Flush 在流结束时调用，把未完成的缓冲原样放出。
func (f *citationFilter) Flush() string {
	return f.process(true)
}

// Dropped 返回被丢弃的越界引用标记数量。
func (f *citationFilter) Dropped() int { return f.dropped }

func (f *citationFilter) process(final bool) string {
	var out strings.Builder
	i := 0
	for i < len(f.pending) {
		r := f.pending[i]
		if r != '[' {
			out.WriteRune(r)
			i++
			continue
		}

		// "[" 后尚未到达下一个字符：不确定是否为引用标记，等下一段。
		if i+1 >= len(f.pending) {
			if !final {
				break
			}
			out.WriteRune(r)
			i++
			continue
		}

		// 非数字开头（如 "[注]"、Markdown 链接）：按普通文本放行。
		if !isDigit(f.pending[i+1]) {
			out.WriteRune(r)
			i++
			continue
		}

		j := i + 1
		for j < len(f.pending) && isDigit(f.pending[j]) {
			j++
		}
		digits := j - (i + 1)

		// 数字太多：不可能是合法引用，逐字放行，避免缓冲无限增长。
		if digits > maxCitationDigits {
			out.WriteRune(r)
			i++
			continue
		}

		// 数字已到缓冲末尾：等待后续字符（可能是更多数字或 "]"）。
		if j >= len(f.pending) {
			if !final {
				break
			}
			out.WriteString(string(f.pending[i:]))
			i = len(f.pending)
			continue
		}

		// 数字后不是 "]"，说明不是引用标记（如 "[1,2]"）：放行 "["，后续按普通文本处理。
		if f.pending[j] != ']' {
			out.WriteRune(r)
			i++
			continue
		}

		n, _ := strconv.Atoi(string(f.pending[i+1 : j]))
		if n >= 1 && n <= f.max {
			out.WriteString(string(f.pending[i : j+1]))
		} else {
			f.dropped++
		}
		i = j + 1
	}

	f.pending = append([]rune(nil), f.pending[i:]...)
	return out.String()
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
