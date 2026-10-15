// 把生物学语料灌入知识库（POST /api/materials，以教师身份触发建索引）。
//
//   node seed.mjs                        # 默认 http://127.0.0.1:8080，teacher1/teacher123
//   BASE=... USER=... PASS=... node seed.mjs
//
// 语料来自 corpus.json（由 fetch-from-wikipedia.mjs 从中文维基百科抓取生成）。
// 幂等：按材料标题跳过班级内已存在的材料，可重复执行。
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const BASE = process.env.BASE || 'http://127.0.0.1:8080'
const USER = process.env.USER || 'teacher1'
const PASS = process.env.PASS || 'teacher123'
const HERE = dirname(fileURLToPath(import.meta.url))

const corpus = JSON.parse(readFileSync(new URL('./corpus.json', import.meta.url), 'utf8'))

// 中文维基个别条目标题为繁体或地域变体，展示时用大陆通用词
const DISPLAY = {
  演化: '进化',
  食物鏈: '食物链',
  卡爾文循環: '卡尔文循环',
  線粒體: '线粒体',
  細胞週期: '细胞周期',
  有絲分裂: '有丝分裂',
  葉: '叶',
  種子: '种子',
}

const pieces = []
for (const entry of corpus) {
  const title = DISPLAY[entry.pageTitle] || entry.pageTitle
  entry.bags.forEach((text, i) => {
    pieces.push({
      domain: entry.domain,
      source: entry.pageTitle,
      title: entry.bags.length > 1 ? `${entry.domain}｜${title}（${i + 1}/${entry.bags.length}）` : `${entry.domain}｜${title}`,
      text,
    })
  })
}
console.log(`语料：${pieces.length} 条材料，来自 ${new Set(pieces.map((p) => p.domain)).size} 个领域`)

let token = ''
async function login() {
  const r = await fetch(`${BASE}/api/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS }),
  })
  if (!r.ok) throw new Error(`登录失败 ${r.status}`)
  token = (await r.json()).access_token
}

async function api(path, opts = {}) {
  const r = await fetch(BASE + path, {
    ...opts,
    headers: { Authorization: `Bearer ${token}`, ...(opts.headers || {}) },
  })
  if (r.status === 401 && !opts.__retried) {
    await login()
    return api(path, { ...opts, __retried: true })
  }
  return r
}

await login()

// 已存在的材料标题（本班），用于幂等跳过
const existing = new Set()
{
  const r = await api('/api/materials')
  if (r.ok) {
    const j = await r.json()
    for (const m of j.materials || []) existing.add(m.title)
  }
}
console.log(`班级内已有材料 ${existing.size} 条`)

let ok = 0, skipped = 0, failed = 0
const manifest = []
for (const p of pieces) {
  if (existing.has(p.title)) {
    skipped++
    manifest.push({ ...p, status: 'skipped' })
    continue
  }
  const r = await api('/api/materials', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title: p.title, content: p.text }),
  })
  const body = await r.json().catch(() => ({}))
  if (r.status === 201 && body.index_status === 'ready') {
    ok++
    console.log(`  ✓ ${p.title}（${p.text.length} 字）→ id=${body.id} ready`)
    manifest.push({ ...p, id: body.id, status: 'ready' })
  } else {
    failed++
    console.log(`  ✗ ${p.title} → ${r.status} ${JSON.stringify(body).slice(0, 160)}`)
    manifest.push({ ...p, status: 'failed', detail: body })
  }
}
writeFileSync(new URL('./manifest.json', import.meta.url), JSON.stringify(manifest, null, 1), 'utf8')
console.log(`\n== 完成：新增 ${ok}，跳过 ${skipped}，失败 ${failed} ==`)
process.exit(failed > 0 ? 1 : 0)
