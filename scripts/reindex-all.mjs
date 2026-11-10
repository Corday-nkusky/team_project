// 把存量材料按当前 RAG 链路重新切片并重新嵌入。
//
// 背景：链路换了（结构切片 + heading_path + 路径前缀嵌入），或嵌入模型换了
// （course-embedding → embedding-3，同为 2048 维但向量空间不同，必须全量覆盖），
// 但 MySQL 里的 chunks 与 Qdrant 里的点都是旧产物；服务端只在「上传材料」或
// 「单篇 reindex」时建索引，启动时不会自动补齐存量数据。
//
// 异步契约：reindex 接口返回 202 + task_id + status_url，本脚本轮询只读状态接口
// 到终态，succeeded 才算完成；请求内不再等待建索引，也不会拿到 ready。
//
// 用法：
//   node scripts/reindex-all.mjs                     # 全量（默认只补未重建的）
//   node scripts/reindex-all.mjs --limit 2          # 只跑 2 篇，用于验证
//   node scripts/reindex-all.mjs --all              # 连已重建的也重跑
//   node scripts/reindex-all.mjs --reset            # 清空本机进度后重跑（换模型后用）
//   node scripts/reindex-all.mjs --delay-ms 500     # 控制对网关的压力
//
// 环境变量：BASE / USER / PASS（默认 http://127.0.0.1:8080、teacher1/teacher123）
//
// 幂等：已完成（任务 succeeded）的材料默认跳过；进度写在
// .workbuddy/reindex-progress.json，中断后重跑自动续。
// 遇到 429 / 额度耗尽 / 连续失败立即停止，避免把网关额度打爆后继续往库里写半截数据。
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
const TIMEOUT = Number(opt('--timeout-ms', '180000'))
const FORCE_ALL = args.includes('--all')
const ONLY_FAILED = args.includes('--only-failed')
const RESET = args.includes('--reset')

if (!Number.isFinite(LIMIT) || LIMIT < 0 || !Number.isFinite(DELAY) || DELAY < 0 ||
    !Number.isFinite(TIMEOUT) || TIMEOUT <= 0) {
  console.error('参数不合法：--limit / --delay-ms / --timeout-ms 必须是正数')
  process.exit(2)
}
if (RESET && existsSync(PROGRESS)) writeFileSync(PROGRESS, '{}', 'utf8')

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

const TERMINAL = new Set(['succeeded', 'failed', 'dead'])
async function waitForTask(task, statusURL) {
  const deadline = Date.now() + TIMEOUT
  while (Date.now() < deadline) {
    const r = await api(statusURL || `/api/tasks/${task}`)
    if (!r.ok) throw new Error(`查询任务状态失败 ${r.status}`)
    const state = await r.json()
    if (TERMINAL.has(state.status)) return state
    await new Promise((resolve) => setTimeout(resolve, 500))
  }
  throw new Error(`任务 ${task} 在 ${TIMEOUT}ms 内未到终态（worker 可能未启动）`)
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
    const accepted = await r.json().catch(() => ({}))
    if (r.status !== 202 || !accepted.task_id) {
      throw new Error(`${r.status} ${JSON.stringify(accepted).slice(0, 160)}`)
    }
    const state = await waitForTask(accepted.task_id, accepted.status_url)
    if (state.status === 'succeeded') {
      done[m.id] = 'ok'
      ok++
      consecutiveFailures = 0
      console.log(`  ✓ id=${m.id} ${m.title}（任务 ${accepted.task_id.slice(0, 8)}，${state.attempts} 次尝试）`)
    } else {
      throw new Error(`任务 ${state.status}: ${(state.error || '').slice(0, 200)}`)
    }
  } catch (e) {
    failed++
    consecutiveFailures++
    const msg = e.message
    console.log(`  ✗ id=${m.id} ${m.title} → ${msg}`)
    if (/429|额度|quota|rate limit|limit/i.test(msg)) {
      console.log('\n网关额度/限流，立即停止；已完成的进度已保存，额度恢复后重跑本脚本即可续。')
      break
    }
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
