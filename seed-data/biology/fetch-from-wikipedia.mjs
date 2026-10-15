// 从中文维基百科抓取生物学语料：搜索定位正确标题（避开消歧义页）→ 逐条抓全文 → 按 2400 字装袋成材料
// 用法：PROXY=http://127.0.0.1:<系统代理端口> node fetch-from-wikipedia.mjs
// 境内访问维基需要走系统代理；代理端口会变，用 PROXY 环境变量传入。
import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const PROXY = process.env.PROXY || 'http://127.0.0.1:10909'
const UA = 'CampusClawCourseBot/1.0 (university course project)'
const OUT = fileURLToPath(new URL('./corpus.json', import.meta.url))
const MAX_PIECES = 3          // 每个条目最多切几袋（防止「病毒」「哺乳动物」过长）
const BAG = 2400              // 每袋目标字数

const DOMAINS = {
  生物化学: ['酶', '糖酵解', '三羧酸循环', '糖异生', '蛋白质', '核酸', '卡尔文循环', '脂肪酸'],
  细胞生物学: ['细胞', '细胞膜', '细胞核', '核糖体', '细胞凋亡', '减数分裂', '线粒体', '叶绿体', '细胞周期', '有丝分裂'],
  微生物学: ['微生物', '细菌', '病毒', '真菌', '酵母菌'],
  植物学: ['植物', '光合作用', '根', '花', '叶', '种子'],
  动物学: ['脊椎动物', '无脊椎动物', '哺乳动物', '昆虫', '鸟类', '鱼类'],
  动物行为学: ['动物行为学', '鸟类迁徙', '求偶'],
  遗传学: ['基因', '染色体', '孟德尔定律'],
  进化生物学: ['进化', '自然选择'],
  生态学: ['生态系统', '食物链'],
}

const SKIP = /^(参见|参考文献|外部链接|延伸阅读|注释|出处|参考|外部鏈結|參見|參考文獻|延伸閱讀|注釋|書目|书目)$/

const blockSleep = (ms) => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms)

function api(params) {
  const args = ['-s', '-x', PROXY, '-m', '45', '-A', UA]
  for (const [k, v] of Object.entries(params)) args.push('--data-urlencode', `${k}=${v}`)
  args.push('https://zh.wikipedia.org/w/api.php')
  for (let i = 0; i < 6; i++) {
    let out
    try {
      out = execFileSync('curl', args, { maxBuffer: 64 * 1024 * 1024, encoding: 'utf8' })
    } catch (e) {
      blockSleep(3000 * (i + 1))   // 代理瞬时故障（SSL/超时），退避重试
      continue
    }
    if (/too many requests/i.test(out)) { blockSleep(5000 * (i + 1)); continue }
    try { return JSON.parse(out) } catch { blockSleep(4000); continue }
  }
  return null
}

function searchTitles(kw) {
  const j = api({ action: 'query', format: 'json', list: 'search', srlimit: '6', srnamespace: '0', srsearch: kw })
  return (j?.query?.search || []).map((s) => s.title)
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
    if (line.length <= 12 && !/[。，；、？！]/.test(line)) continue  // 光秃小节名 / 公式碎片
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

// 断点续抓：读入已有结果（仅复用带 keyword 标记的），中断后重跑不会重抓
let result = []
try {
  result = JSON.parse(readFileSync(OUT, 'utf8')).filter((x) => x.keyword)
} catch { result = [] }
const doneKeys = new Set(result.map((r) => `${r.domain}|${r.keyword}`))
if (result.length) console.log(`复用已抓 ${result.length} 条`)

for (const [domain, kws] of Object.entries(DOMAINS)) {
  for (const kw of kws) {
    if (doneKeys.has(`${domain}|${kw}`)) { console.log(`SKIP ${domain} ${kw}（已抓）`); continue }
    let page = null
    const cands = searchTitles(kw)
    for (const t of cands) {
      page = fetchPage(t)
      if (page && page.text.length >= 900) break   // 排除消歧义页/重定向壳
      page = null
      blockSleep(1200)
    }
    if (!page) { console.log(`MISS ${domain} ${kw}（候选 ${cands.join(', ')}）`); blockSleep(1200); continue }
    const bags = toBags(page.text)
    if (!bags.length) { console.log(`EMPTY ${domain} ${page.title}`); blockSleep(1200); continue }
    console.log(`OK ${domain}｜${page.title} 全文${page.text.length}字 -> ${bags.length} 袋（${bags.map((b) => b.length).join('/')}）`)
    result.push({ domain, keyword: kw, pageTitle: page.title, bags })
    doneKeys.add(`${domain}|${kw}`)
    writeFileSync(OUT, JSON.stringify(result, null, 1))   // 每抓完一条即落盘
    blockSleep(1200)
  }
}
writeFileSync(OUT, JSON.stringify(result, null, 1))
console.log('\n条目:', result.length, ' 材料袋:', result.reduce((a, b) => a + b.bags.length, 0))
