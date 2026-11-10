// 从中文维基百科抓取《原神》角色条目作为演示语料：搜索定位正确标题（避开消歧义页）→ 抓全文 → 按 2400 字装袋。
// 用法：PROXY=http://127.0.0.1:<系统代理端口> node fetch-from-wikipedia.mjs
// 境内访问维基需要走系统代理；代理端口会变，用 PROXY 环境变量传入。
// 语料来源为中文维基百科，遵循 CC BY-SA 授权；抓的是「条目正文」，不是游戏内原文。
import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const PROXY = process.env.PROXY || 'http://127.0.0.1:10909'
const UA = 'CampusClawCourseBot/1.0 (university course project)'
const OUT = fileURLToPath(new URL('./corpus.json', import.meta.url))
const MAX_PIECES = 3          // 每个条目最多切几袋
const BAG = 2400              // 每袋目标字数

const DOMAIN = '角色设定'

// 只认精确条目名：模糊搜索「流浪者」会命中「露宿者」、「旅行者」会命中「旅行者1号」、「胡桃」会命中核桃树。
const TARGETS = [
  '派蒙 (原神)', '钟离 (原神)', '温迪 (原神)', '琴 (原神)', '胡桃 (原神)', '可莉 (原神)',
  '甘雨 (原神)', '魈 (原神)', '流浪者 (原神)', '旅行者 (原神)',
]
const keywordOf = (t) => t.replace(/\s*\(原神\)$/, '')

// 抓完校验：正文开篇必须是在讲《原神》这只角色，防止消歧义/同名条目混进来
const isGenshin = (text) => /原神|米哈游/.test(text.slice(0, 300))

const SKIP = /^(参见|参考文献|外部链接|延伸阅读|注释|出处|参考|外部鏈結|參見|參考文獻|延伸閱讀|注釋|書目|书目|角色相关|相关条目)$/

const blockSleep = (ms) => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms)

function api(params) {
  const args = ['-s', '-x', PROXY, '-m', '45', '-A', UA]
  for (const [k, v] of Object.entries(params)) args.push('--data-urlencode', `${k}=${v}`)
  args.push('https://zh.wikipedia.org/w/api.php')
  for (let i = 0; i < 6; i++) {
    let out
    try {
      out = execFileSync('curl', args, { maxBuffer: 64 * 1024 * 1024, encoding: 'utf8' })
    } catch {
      blockSleep(3000 * (i + 1))   // 代理瞬时故障（SSL/超时），退避重试
      continue
    }
    if (/too many requests/i.test(out)) { blockSleep(5000 * (i + 1)); continue }
    try { return JSON.parse(out) } catch { blockSleep(4000); continue }
  }
  return null
}

function fetchPage(title) {
  const j = api({ action: 'query', format: 'json', prop: 'extracts', explaintext: '1', redirects: '1', variant: 'zh-cn', titles: title })
  const p = Object.values(j?.query?.pages || {})[0]
  return p && p.extract ? { title: p.title, text: p.extract } : null
}

function clean(t) {
  return t
    .replace(/^\s*簡繁重定向：[\s\S]*?(?=\n\n)/, ' ')
    .replace(/\{\{[^{}]*\}\}/g, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/\[\[文件:[^\]]*\]\]/g, ' ')
    .replace(/\[\[[^\]|]*\|([^\]]*)\]\]/g, '$1')
    .replace(/\[\[([^\]]*)\]\]/g, '$1')
    .replace(/[ \t]+/g, ' ')
    .trim()
}

function toBags(text) {
  const paras = []
  let skipping = false
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    if (/[{}]|\\(mathrm|displaystyle|ce|quad|overrightarrow|frac|text|begin|end|over|sum)/.test(line)) continue  // LaTeX / 公式残留
    const m = /^={2,}\s*(.+?)\s*={2,}$/.exec(line)
    if (m) {
      skipping = SKIP.test(m[1].trim())
      continue                                  // 小节名本身不进材料
    }
    if (skipping) continue
    if (line.length <= 12 && !/[。，；、？！]/.test(line)) continue  // 光秃小节名 / 碎片
    paras.push(line)
  }
  const body = clean(paras.join('\n\n'))
  const bags = []
  let bag = ''
  for (const para of body.split('\n\n')) {
    const p = para.trim()
    if (!p) continue
    if (bag && bag.length + p.length > BAG) { bags.push(bag.trim()); bag = '' }
    bag += (bag ? '\n\n' : '') + p
  }
  if (bag.trim()) bags.push(bag.trim())
  // 过短的尾袋并回上一袋
  if (bags.length > 1 && bags[bags.length - 1].length < 600) {
    bags[bags.length - 2] += '\n\n' + bags.pop()
  }
  return bags.filter((b) => b.length >= 300).slice(0, MAX_PIECES)
}

// 断点续抓：读入已有结果，中断后重跑不会重抓
let result = []
try {
  result = JSON.parse(readFileSync(OUT, 'utf8')).filter((x) => x.keyword)
} catch { result = [] }
const doneKeys = new Set(result.map((r) => `${r.domain}|${r.keyword}`))
if (result.length) console.log(`复用已抓 ${result.length} 条`)

for (const t of TARGETS) {
  const kw = keywordOf(t)
  if (doneKeys.has(`${DOMAIN}|${kw}`)) { console.log(`SKIP ${t}（已抓）`); continue }
  const page = fetchPage(t)
  if (!page || page.text.length < 900) { console.log(`MISS ${t}`); blockSleep(1200); continue }
  if (!isGenshin(page.text)) { console.log(`REJECT ${t}（开篇不是原神角色）`); blockSleep(1200); continue }
  const bags = toBags(page.text)
  if (!bags.length) { console.log(`EMPTY ${page.title}`); blockSleep(1200); continue }
  console.log(`OK ${page.title} 全文${page.text.length}字 -> ${bags.length} 袋（${bags.map((b) => b.length).join('/')}）`)
  result.push({ domain: DOMAIN, keyword: kw, pageTitle: kw, bags })
  doneKeys.add(`${DOMAIN}|${kw}`)
  writeFileSync(OUT, JSON.stringify(result, null, 1))   // 每抓完一条即落盘
  blockSleep(1200)
}
writeFileSync(OUT, JSON.stringify(result, null, 1))
console.log('\n条目:', result.length, ' 材料袋:', result.reduce((a, b) => a + b.bags.length, 0))
