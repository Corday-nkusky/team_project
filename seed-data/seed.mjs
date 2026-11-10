// 把演示语料灌入知识库（POST /api/materials → 202 + 轮询索引任务）。
//
//   node seed.mjs                        # 默认语料 biology，http://127.0.0.1:8080，teacher1/teacher123
//   node seed.mjs --corpus genshin       # 换成《原神》角色语料
//   node seed.mjs --no-wait              # 只提交，不等索引完成就退出
//   BASE=... USER=... PASS=... node seed.mjs
//
// 每个语料目录约定：corpus.json（由该目录的 fetch-from-wikipedia.mjs 生成）、可选 display.json
//（条目标题的繁简/改写表）、输出 manifest.json。
//
// 幂等且可自愈：标题已存在且已 ready 的跳过；已存在但未 ready 的提交重索引；
// 不存在的新建。因此额度耗尽（429）导致的中断可以直接重跑，不会重复建材料。
import { existsSync, readFileSync, writeFileSync } from 'node:fs'

const BASE = process.env.BASE || 'http://127.0.0.1:8080'
const USER = process.env.USER || 'teacher1'
const PASS = process.env.PASS || 'teacher123'
const NO_WAIT = process.argv.includes('--no-wait')
const POLL_MS = Number(process.env.POLL_MS || 800)
const TIMEOUT_MS = Number(process.env.SEED_TIMEOUT_MS || 30 * 60 * 1000)

const argvCorpus = (() => {
  const i = process.argv.indexOf('--corpus')
  return i > -1 ? process.argv[i + 1] : null
})()
const CORPUS = argvCorpus || process.env.CORPUS || 'biology'
const CORPUS_DIR = new URL(`./${CORPUS}/`, import.meta.url)
if (!existsSync(CORPUS_DIR)) throw new Error(`语料目录不存在：seed-data/${CORPUS}/`)
const OUT_MANIFEST = new URL('./manifest.json', CORPUS_DIR)

const TERMINAL = new Set(['succeeded', 'failed', 'dead'])
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

const corpus = JSON.parse(readFileSync(new URL('./corpus.json', CORPUS_DIR), 'utf8'))

// 中文维基个别条目标题为繁体或地域变体，展示时用大陆通用词（按语料目录可选）
const DISPLAY = existsSync(new URL('./display.json', CORPUS_DIR))
  ? JSON.parse(readFileSync(new URL('./display.json', CORPUS_DIR), 'utf8'))
  : {}

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

// 本班已有材料：标题 → { id, index_status }，用于幂等跳过与失败自愈。
const existing = new Map()
{
  const r = await api('/api/materials')
  if (!r.ok) throw new Error(`读取材料列表失败 ${r.status}`)
  const j = await r.json()
  for (const m of j.materials || []) existing.set(m.title, { id: m.id, index_status: m.index_status })
}
const readyCount = [...existing.values()].filter((m) => m.index_status === 'ready').length
console.log(`班级内已有材料 ${existing.size} 条（其中 ${readyCount} 条已可检索）`)

let skipped = 0
let rejected = 0
const submitted = []
const manifest = []
// manifest 只记录元信息（不含正文），避免把 1MB 语料重复写一遍。
const row = (p, extra) => ({ domain: p.domain, source: p.source, title: p.title, ...extra })

for (const [i, p] of pieces.entries()) {
  const hit = existing.get(p.title)
  if (hit && hit.index_status === 'ready') {
    skipped++
    manifest.push(row(p, { id: hit.id, status: 'skipped' }))
    continue
  }

  let r
  if (hit) {
    // 材料在库但索引未完成（曾经失败/中断）：提交重索引，不重复建材料。
    r = await api(`/api/materials/${hit.id}/reindex`, { method: 'POST' })
  } else {
    r = await api('/api/materials', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: p.title, content: p.text }),
    })
  }
  const body = await r.json().catch(() => ({}))

  if (r.status !== 202 || !body.status_url) {
    rejected++
    console.log(`  ✗ ${p.title} → ${r.status} ${JSON.stringify(body).slice(0, 160)}`)
    manifest.push(row(p, { id: hit?.id, status: 'rejected', detail: body }))
    continue
  }

  submitted.push({
    ...p,
    materialID: body.material_id,
    taskID: body.task_id,
    statusURL: body.status_url,
    state: body.status,
    repaired: Boolean(hit),
  })
  if ((i + 1) % 20 === 0 || i === pieces.length - 1) {
    console.log(`  进度 ${i + 1}/${pieces.length}（新受理 ${submitted.length}，跳过 ${skipped}）`)
  }
}

console.log(`\n提交完成：新受理 ${submitted.length}，跳过 ${skipped}，拒绝 ${rejected}`)

// 轮询所有任务到终态。status_url 只读、按班级隔离，跨班会 404。
const finished = new Map()
let pending = submitted.filter((s) => !TERMINAL.has(s.state))
for (const s of submitted) if (TERMINAL.has(s.state)) finished.set(s.taskID, { status: s.state })

if (NO_WAIT) {
  for (const s of pending) finished.set(s.taskID, { status: 'submitted' })
  pending = []
  console.log('已跳过等待（--no-wait）：索引由 worker 继续执行')
} else if (pending.length) {
  console.log(`等待 ${pending.length} 个索引任务完成（超时 ${Math.round(TIMEOUT_MS / 60000)} 分钟）…`)
  const t0 = Date.now()
  while (pending.length) {
    if (Date.now() - t0 > TIMEOUT_MS) break
    const still = []
    for (const s of pending) {
      const r = await api(s.statusURL)
      if (!r.ok) {
        still.push(s)
        continue
      }
      const st = await r.json()
      if (TERMINAL.has(st.status)) finished.set(s.taskID, st)
      else still.push(s)
    }
    pending = still
    if (pending.length) {
      const sec = ((Date.now() - t0) / 1000).toFixed(0)
      console.log(`  进行中：剩 ${pending.length}/${submitted.length}（${sec}s）`)
      await sleep(POLL_MS)
    }
  }
  if (pending.length) console.log(`  ⚠ 超时：仍有 ${pending.length} 个任务未完成，worker 可能未启动`)
}

let ok = 0
let failed = 0
let accepted = 0
const failures = []
const recorded = new Set(manifest.map((m) => m.title))
for (const s of submitted) {
  const st = finished.get(s.taskID) || { status: 'timeout' }
  const line = row(s, { id: s.materialID, task_id: s.taskID })
  recorded.add(s.title)
  if (st.status === 'succeeded') {
    ok++
    manifest.push({ ...line, status: 'ready' })
  } else if (st.status === 'submitted') {
    accepted++
    manifest.push({ ...line, status: 'submitted' })
  } else {
    failed++
    failures.push({ title: s.title, status: st.status, error: st.error || '' })
    manifest.push({ ...line, status: st.status, error: st.error || '' })
  }
}
for (const p of pieces) {
  if (!recorded.has(p.title)) manifest.push(row(p, { status: 'skipped' }))
}

writeFileSync(OUT_MANIFEST, JSON.stringify(manifest, null, 1), 'utf8')

if (failures.length) {
  console.log('\n失败明细（前 5 条）：')
  for (const f of failures.slice(0, 5)) console.log(`  ✗ ${f.title} → ${f.status} ${f.error.slice(0, 120)}`)
  const throttled = failures.some((f) => /429|额度|限流|rate limit/i.test(f.error))
  if (throttled) {
    console.log('\n检测到 429 / 额度限制：等额度恢复后重跑本脚本即可，已 ready 的材料会自动跳过。')
  }
  if (failures.some((f) => f.status === 'timeout')) {
    console.log('有任务停在 pending：确认 worker 容器在跑（docker compose ps worker）。')
  }
}

const total = skipped + ok + accepted
console.log(`\n== 完成：可检索 ${ok}，新受理待索引 ${accepted}，跳过 ${skipped}，拒绝 ${rejected}，失败 ${failed} ==`)
console.log(`   班级内合计 ${skipped + ok} 条材料可检索（脚本按标题幂等，可重复执行）`)
process.exit(failed + rejected > 0 || total < pieces.length ? 1 : 0)
