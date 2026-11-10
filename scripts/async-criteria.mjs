#!/usr/bin/env node
// 第 8 课「异步任务与消息队列」三条验收判据的可复跑复现脚本。
//
//   node scripts/async-criteria.mjs                    # 三条全跑
//   node scripts/async-criteria.mjs --criterion 2     # 只跑判据二
//   node scripts/async-criteria.mjs --count 5         # 判据一提交次数改为 5
//
// 前置：docker compose 已起来（node scripts/up.mjs）、deploy/.env 可读、
//       本机 docker CLI 可用。脚本会 stop / kill / start worker，结束时保证恢复。
// 输出：标准输出 + reports/async-criteria.md（命令、原始输出、结论），不写死结论。
//
// 判据一：提交 P95 ≪ 一次索引执行耗时，且响应不含结果。
// 判据二：停消费者后提交，任务停在 pending；恢复后自动执行不丢。
// 判据三：重复提交 / 重复投递只出一份结果（每个 seq 一行切片、向量点数不翻倍）。
import { spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { performance } from 'node:perf_hooks'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const envText = readFileSync(path.join(root, 'deploy/.env'), 'utf8')
const cfg = {}
for (const line of envText.split(/\r?\n/)) {
  if (!line || line.startsWith('#') || !line.includes('=')) continue
  const [k, ...v] = line.split('=')
  cfg[k.trim()] = v.join('=').trim()
}
const BASE = process.env.BASE || 'http://127.0.0.1:8080'
const USER = process.env.USER || 'teacher1'
const PASS = process.env.PASS || 'teacher123'
const COMPOSE = ['compose', '-f', 'deploy/docker-compose.yml']

const args = process.argv.slice(2)
const opt = (name, fallback) => {
  const i = args.indexOf(name)
  return i >= 0 && args[i + 1] ? args[i + 1] : fallback
}
const WHICH = opt('--criterion', 'all')
const COUNT = Number(opt('--count', '20'))
if (!Number.isInteger(COUNT) || COUNT < 1 || COUNT > 100) {
  console.error('--count 必须是 1..100 的整数')
  process.exit(2)
}

const lines = []
const say = (s = '') => { console.log(s); lines.push(s) }
const section = (s) => say(`\n## ${s}\n`)
const command = (s) => say(`$ ${s}`)
const output = (s) => {
  const t = String(s ?? '').trim()
  if (t) say('```\n' + t.slice(0, 4000) + '\n```')
}
say(`# 第 8 课异步三判据复现记录`)
say(`\n- 时间：${new Date().toISOString()}`)
say(`- 服务：${BASE}；compose：-f deploy/docker-compose.yml`)
say(`- 参数：--criterion ${WHICH} --count ${COUNT}`)

const results = {}
const verdict = (name, pass, detail) => {
  results[name] = pass === null ? 'SKIP' : pass ? 'PASS' : 'FAIL'
  say(`**结论：${results[name]}** — ${name}：${detail}\n`)
}

function compose(...rest) {
  const cmd = ['docker', ...COMPOSE, ...rest]
  command(cmd.join(' '))
  const r = spawnSync(cmd[0], cmd.slice(1), { cwd: root, encoding: 'utf8', timeout: 120000 })
  const text = ((r.stdout || '') + (r.stderr || '')).trim()
  output(text)
  return { ok: r.status === 0, text }
}

function mysql(sql) {
  const cmd = ['docker', ...COMPOSE, 'exec', '-T', '-e', `MYSQL_PWD=${cfg.MYSQL_PASSWORD}`,
    'mysql', 'mysql', '-u', cfg.MYSQL_USER, '-N', '-B', cfg.MYSQL_DATABASE, '-e', sql]
  command('docker compose exec -T mysql mysql ... -e "' + sql.replace(/\s+/g, ' ').slice(0, 220) + '"')
  const r = spawnSync(cmd[0], cmd.slice(1), { cwd: root, encoding: 'utf8', timeout: 60000 })
  const text = ((r.stdout || '') + (r.stderr || '')).trim()
  output(text)
  return text
}

function qdrantCount(materialID) {
  const body = JSON.stringify({ exact: true, filter: { must: [{ key: 'material_id', match: { any: [materialID] } }] } })
  const sh = `wget -qO- --header='Content-Type: application/json' --post-data='${body}' ` +
    'http://qdrant:6333/collections/team_campusclaw_chunks/points/count'
  command(`docker compose exec -T backend sh -c "wget ... /points/count (material_id=${materialID})"`)
  const r = spawnSync('docker', [...COMPOSE, 'exec', '-T', 'backend', 'sh', '-c', sh],
    { cwd: root, encoding: 'utf8', timeout: 30000 })
  const text = ((r.stdout || '') + (r.stderr || '')).trim()
  output(text)
  const m = text.match(/"count"\s*:\s*(\d+)/)
  return m ? Number(m[1]) : null
}

function qdrantDelete(materialID) {
  const body = JSON.stringify({ filter: { must: [{ key: 'material_id', match: { any: [materialID] } }] } })
  const sh = `wget -qO- --header='Content-Type: application/json' --post-data='${body}' ` +
    'http://qdrant:6333/collections/team_campusclaw_chunks/points/delete?wait=true'
  spawnSync('docker', [...COMPOSE, 'exec', '-T', 'backend', 'sh', '-c', sh],
    { cwd: root, encoding: 'utf8', timeout: 30000 })
}

let token = ''
async function login() {
  const r = await fetch(`${BASE}/api/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS }),
  })
  if (!r.ok) throw new Error(`登录失败 ${r.status}`)
  token = (await r.json()).access_token
}
async function api(p, init = {}) {
  const r = await fetch(BASE + p, {
    ...init, headers: { Authorization: `Bearer ${token}`, ...(init.headers || {}) },
  })
  let body = null
  try { body = await r.json() } catch { body = {} }
  return { status: r.status, body, raw: JSON.stringify(body) }
}
const TERMINAL = new Set(['succeeded', 'failed', 'dead'])
async function waitTask(taskID, timeoutMs = 120000, onState) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const r = await api(`/api/tasks/${taskID}`)
    if (r.status === 200 && onState) onState(r.body)
    if (r.status === 200 && TERMINAL.has(r.body.status)) return r.body
    await new Promise((res) => setTimeout(res, onState ? 60 : 300))
  }
  return null
}
const sleep = (ms) => new Promise((res) => setTimeout(res, ms))

const created = []
const unique = Date.now().toString(36)
async function submit(title, content) {
  const t0 = performance.now()
  const r = await api('/api/materials', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, content }),
  })
  const elapsed = performance.now() - t0
  if (r.status === 202 && r.body.material_id) created.push(r.body.material_id)
  return { ...r, elapsed }
}

// ---------------------------------------------------------------- 判据一
async function criterion1() {
  section('判据一 · 提交 P95 ≪ 执行耗时，且响应不含结果')
  const times = []
  const tasks = []
  let allAccepted = true, noResult = true, firstTask = null
  for (let i = 0; i < COUNT; i++) {
    const r = await submit(`判据一-${unique}-${i}`, `第 ${i} 份异步验收材料，独有词 ${unique}A${i}。索引由独立消费者执行。`)
    times.push(r.elapsed)
    if (r.status !== 202 || !r.body.task_id || !r.body.status_url) allAccepted = false
    if (/ready|chunk|citations/i.test(r.raw)) noResult = false
    if (r.body.task_id) tasks.push(r.body.task_id)
    if (!firstTask) firstTask = r.body
  }
  times.sort((a, b) => a - b)
  const p95 = times[Math.min(times.length - 1, Math.floor(times.length * 0.95))]
  const state = firstTask ? await waitTask(firstTask.task_id, 150000) : null
  // 把本批任务都等到终态：否则停止消费者时残留的 pending 任务会干扰判据二/三的时序。
  let settled = 0
  for (const id of tasks) {
    const s = await waitTask(id, 150000)
    if (s && TERMINAL.has(s.status)) settled++
  }
  say(`- 本批 ${tasks.length} 个任务到达终态：${settled}`)
  let execMs = null
  if (state?.started_at && state?.finished_at) {
    execMs = new Date(state.finished_at) - new Date(state.started_at)
  }
  command(`连续 POST /api/materials ${COUNT} 次（curl -w '%{time_total}' 等价：脚本内 performance.now()）`)
  say(`- 每次提交耗时（ms，升序）：${times.map((t) => t.toFixed(1)).join(', ')}`)
  say(`- P95 = **${p95.toFixed(1)} ms**；首个任务状态 = ${state?.status ?? '未到终态'}`)
  say(`- 一次索引执行耗时（finished_at − started_at）= ${execMs === null ? '未采集' : execMs + ' ms'}`)
  say(`- 202 且含 task_id / status_url：${allAccepted}；响应不含索引结果：${noResult}`)
  verdict('判据一', allAccepted && noResult && execMs !== null && p95 * 5 < execMs,
    `P95 ${p95.toFixed(1)}ms vs 执行 ${execMs ?? '未采集'}ms；不含结果=${noResult}。` +
    (execMs === null ? ' 任务未到终态，判据一记为 FAIL（不假装通过）。' : ''))
}

// ---------------------------------------------------------------- 判据二
async function criterion2() {
  section('判据二 · 停消费者提交、恢复后自动执行不丢')
  compose('stop', 'worker')
  try {
    const r = await submit(`判据二-${unique}`, `停止消费者期间提交的材料，独有词 ${unique}B。`)
    command(`POST /api/materials → ${r.status} ${r.raw}`)
    await sleep(2000)
    const pending = await api(`/api/tasks/${r.body.task_id}`)
    say(`- 消费者停止 2 秒后查询：${pending.status} ${JSON.stringify(pending.body)}`)
    compose('start', 'worker')
    const state = await waitTask(r.body.task_id, 150000)
    say(`- 恢复消费者后终态：${state ? `${state.status} attempts=${state.attempts}` : '超时'}`)
    verdict('判据二', r.status === 202 && pending.body.status === 'pending' && state?.status === 'succeeded',
      `停止期间状态=${pending.body.status}，恢复后=${state?.status ?? '超时'}`)
  } finally {
    compose('start', 'worker')
  }
}

// ---------------------------------------------------------------- 判据三
async function criterion3() {
  section('判据三 · 重复提交 / 重复投递只出一份结果')
  const content = `重复投递验收材料，独有词 ${unique}C。`.repeat(12)
  const a = await submit(`判据三-${unique}`, content)
  const b = await submit(`判据三-${unique}`, content)
  command(`连续 POST 同一份内容两次 → ${a.status} / ${b.status}`)
  say(`- task_id：${a.body.task_id} / ${b.body.task_id}；material_id：${a.body.material_id} / ${b.body.material_id}`)
  const idem = a.body.task_id === b.body.task_id && a.body.material_id === b.body.material_id

  const first = await waitTask(a.body.task_id, 150000)
  const chunks1 = mysql(`SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=${a.body.material_id}`)
  const points1 = qdrantCount(a.body.material_id)

  // 对同一份材料再发起一次重索引：成功后 revision+1 会生成新任务，
  // 重复执行全量替换切片与向量，用来验证确定性点 ID 不会让点数翻倍。
  const re = await api(`/api/materials/${a.body.material_id}/reindex`, { method: 'POST' })
  const second = re.status === 202 ? await waitTask(re.body.task_id, 150000) : null
  const chunks2 = mysql(`SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=${a.body.material_id}`)
  const points2 = qdrantCount(a.body.material_id)
  const [total1, distinct1] = chunks1.split(/\s+/).map(Number)
  const [total2, distinct2] = chunks2.split(/\s+/).map(Number)
  say(`- 首次完成：${first?.status}；切片 ${total1} 行 / ${distinct1} 个 seq；向量点 ${points1 ?? '未采集'}`)
  say(`- 再次重索引：${second?.status}；切片 ${total2} 行 / ${distinct2} 个 seq；向量点 ${points2 ?? '未采集'}`)

  // 在 running 期间终止消费者：注入租约过期只是把「等 5 分钟自动到期」压缩到可复跑的时长，
  // 回收动作仍由 worker 启动时的 RecoverStaleTasks 完成，不改业务代码。
  compose('stop', 'worker')
  const c = await submit(`判据三-重投-${unique}`, `至少一次投递验收，独有词 ${unique}D。`.repeat(12))
  compose('start', 'worker')
  let sawRunning = false
  const t0 = Date.now()
  while (Date.now() - t0 < 30000 && !sawRunning) {
    const s = await api(`/api/tasks/${c.body.task_id}`)
    if (s.body.status === 'running') { sawRunning = true; break }
    if (TERMINAL.has(s.body.status)) break
    await sleep(50)
  }
  let attempts = null
  if (sawRunning) {
    compose('kill', 'worker')
    mysql(`UPDATE index_tasks SET lease_until=DATE_SUB(NOW(3), INTERVAL 1 SECOND) WHERE task_id='${c.body.task_id}' AND status='running'`)
    compose('start', 'worker')
    const st = await waitTask(c.body.task_id, 150000)
    attempts = st?.attempts ?? null
    say(`- 捕获 running 后 kill worker，注入租约过期再启动；终态 ${st?.status}，attempts=${attempts}`)
  } else {
    say('- 未捕获到 running 窗口（任务太快完成），重复投递子项记为 SKIP，不伪造结果。')
    compose('start', 'worker')
  }
  const redelivery = sawRunning ? attempts >= 2 : null
  const noDup = total1 === distinct1 && total2 === distinct2 && total1 === total2
  const measuredPoints = points1 !== null && points2 !== null
  const noPointDup = measuredPoints && points2 === points1
  // 任一子项未采集就记 SKIP，绝不把「没测到」写成通过。
  const outcome = (!idem || !noDup || redelivery === false || (measuredPoints && !noPointDup))
    ? false
    : (!measuredPoints || redelivery === null) ? null : true
  verdict('判据三', outcome,
    `入口幂等=${idem}；切片每 seq 一行且不增长=${noDup}；向量点不翻倍=${measuredPoints ? noPointDup : '未采集'}；` +
    `重复投递 attempts≥2=${redelivery === null ? 'SKIP' : redelivery}。` +
    (!measuredPoints ? ' Qdrant 点数未采集（容器内网不可达时记为 SKIP，不算通过）。' : ''))
}

// ---------------------------------------------------------------- 主流程
try {
  await login()
  if (WHICH === 'all' || WHICH === '1') await criterion1()
  if (WHICH === 'all' || WHICH === '2') await criterion2()
  if (WHICH === 'all' || WHICH === '3') await criterion3()
} catch (e) {
  say(`\n**脚本异常**：${e.stack || e.message}`)
  results['执行'] = 'FAIL'
} finally {
  compose('start', 'worker')
  // 清理本次写入的测试材料（chunks / index_tasks 有 ON DELETE CASCADE）。
  if (created.length) {
    const ids = [...new Set(created)].join(',')
    mysql(`DELETE FROM materials WHERE id IN (${ids})`)
    for (const id of new Set(created)) qdrantDelete(id)
  }
  section('汇总')
  for (const [k, v] of Object.entries(results)) say(`- ${k}：${v}`)
  const dir = path.join(root, 'reports')
  if (!existsSync(dir)) mkdirSync(dir, { recursive: true })
  writeFileSync(path.join(dir, 'async-criteria.md'), lines.join('\n') + '\n', 'utf8')
  console.log(`\n归档：reports/async-criteria.md`)
  process.exit(Object.values(results).includes('FAIL') ? 1 : 0)
}
