// 把存量材料按当前 RAG 链路重新切片并重新嵌入。
//
// 背景：链路换了（结构切片 + heading_path + 路径前缀嵌入），但 MySQL 里已有的
// chunks 和 Qdrant 里的点都是旧链路产物；服务端只在「上传材料」或「单篇 reindex」
// 时建索引，启动时不会自动补齐存量数据。seed.mjs 按标题幂等跳过，也修不了。
//
// 用法：
//   node scripts/reindex-all.mjs                     # 全量（默认只补未重建的）
//   node scripts/reindex-all.mjs --limit 2          # 只跑 2 篇，用于验证
//   node scripts/reindex-all.mjs --all              # 连已重建的也重跑
//   node scripts/reindex-all.mjs --delay-ms 500     # 控制对网关的压力
//
// 环境变量：BASE / USER / PASS（默认 http://127.0.0.1:8080、teacher1/teacher123）
//
// 幂等：已完成（index_status=ready 且本次检查已带结构路径）的材料默认跳过；
// 进度写在同目录的 .reindex-progress.json，中断后重跑自动续。
// 遇到 429 / 连续失败立即停止，避免把网关额度打爆后继续往库里写半截数据。
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'

// 进度是本机运行状态，放项目数据目录（已在 .git/info/exclude 中忽略），不污染工作区。
const PROGRESS_DIR = new URL('../.workbuddy/', import.meta.url)
const PROGRESS = new URL('./reindex-progress.json', PROGRESS_DIR)
if (!existsSync(PROGRESS_DIR)) mkdirSync(PROGRESS_DIR, { recursive: true })
const BASE = process.env.BASE || 'http://127.0.0.1:8080'
const USER = process.env.USER || 'teacher1'
const PASS = process.env.PASS || 'teacher123'

const args = process.argv.slice(2)
const opt = (name, fallback) => {
  const i = args.indexOf(name)
  return i >= 0 && args[i + 1] ? args[i + 1] : fallback
}
const LIMIT = Number(opt('--limit', '0'))
const DELAY = Number(opt('--delay-ms', '300'))
const FORCE_ALL = args.includes('--all')
const ONLY_FAILED = args.includes('--only-failed')

if (!Number.isFinite(LIMIT) || LIMIT < 0 || !Number.isFinite(DELAY) || DELAY < 0) {
  console.error('参数不合法：--limit / --delay-ms 必须是非负数字')
  process.exit(2)
}

const done = existsSync(PROGRESS) ? JSON.parse(readFileSync(PROGRESS, 'utf8')) : {}
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
const listRes = await api('/api/materials')
if (!listRes.ok) throw new Error(`列出材料失败 ${listRes.status}`)
const materials = (await listRes.json()).materials || []
materials.sort((a, b) => a.id - b.id)

const targets = materials.filter((m) => {
  if (ONLY_FAILED && m.index_status !== 'failed') return false
  if (!FORCE_ALL && done[m.id] === 'ok') return false
  return true
})
const todo = LIMIT > 0 ? targets.slice(0, LIMIT) : targets
console.log(`材料共 ${materials.length} 篇；待重建 ${targets.length} 篇；本次处理 ${todo.length} 篇`)

let ok = 0, failed = 0, consecutiveFailures = 0
for (const m of todo) {
  try {
    const r = await api(`/api/materials/${m.id}/reindex`, { method: 'POST' })
    const body = await r.json().catch(() => ({}))
    if (r.ok && body.index_status === 'ready') {
      done[m.id] = 'ok'
      ok++
      consecutiveFailures = 0
      console.log(`  ✓ id=${m.id} ${m.title}`)
    } else {
      failed++
      consecutiveFailures++
      const msg = `${r.status} ${JSON.stringify(body).slice(0, 160)}`
      console.log(`  ✗ id=${m.id} ${m.title} → ${msg}`)
      if (/429|额度|quota|rate limit/i.test(msg)) {
        console.log('\n网关额度/限流，立即停止；已完成的进度已保存，额度恢复后重跑本脚本即可续。')
        break
      }
    }
  } catch (e) {
    failed++
    consecutiveFailures++
    console.log(`  ✗ id=${m.id} ${m.title} → ${e.message}`)
  }
  writeFileSync(PROGRESS, JSON.stringify(done, null, 1), 'utf8')
  if (consecutiveFailures >= 3) {
    console.log('\n连续 3 篇失败，停止以免继续写半截数据；进度已保存。')
    break
  }
  if (DELAY > 0) await new Promise((resolve) => setTimeout(resolve, DELAY))
}

writeFileSync(PROGRESS, JSON.stringify(done, null, 1), 'utf8')
console.log(`\n== 完成：成功 ${ok}，失败 ${failed}，累计已重建 ${Object.keys(done).length}/${materials.length} 篇 ==`)
process.exit(failed > 0 ? 1 : 0)
