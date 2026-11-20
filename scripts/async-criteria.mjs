#!/usr/bin/env node
// 第 8 课「异步任务与消息队列」三条验收判据的可复跑复现脚本。
//
//   node scripts/async-criteria.mjs                    # 三条全跑
//   node scripts/async-criteria.mjs --criterion concurrent  # 仅并发上传，不启停 worker
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
import { randomUUID } from 'node:crypto'
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
const CRITERIA = ['all', '1', '2', '3', 'concurrent']
if (!CRITERIA.includes(WHICH)) {
  // 未知取值不能静默空跑：否则一条拼写错误会得到「没有 FAIL 所以退出 0」的空报告。
  console.error(`--criterion 只接受 ${CRITERIA.join(' / ')}，收到 ${WHICH}`)
  process.exit(2)
}
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
say(`# ${WHICH === 'concurrent' ? '第 8 课并发上传验收记录' : '第 8 课异步三判据复现记录'}`)
say(`\n- 时间：${new Date().toISOString()}`)
say(`- 服务：${BASE}；compose：-f deploy/docker-compose.yml`)
// --count 只影响判据一，别在只跑并发时打印一个不生效的参数。
say(`- 参数：--criterion ${WHICH}${WHICH === 'all' || WHICH === '1' ? ` --count ${COUNT}` : ''}`)

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
    'mysql', 'mysql', '--default-character-set=utf8mb4', '-u', cfg.MYSQL_USER, '-N', '-B', cfg.MYSQL_DATABASE, '-e', sql]
  command('docker compose exec -T mysql mysql ... -e "' + sql.replace(/\s+/g, ' ') + '"')
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

// 访问日志里同一 request_id 的行带服务端状态与耗时，是并发判据能取到的最强服务端证据。
// 取不到（容器日志不可达或已滚动）不算失败，由调用方决定如何记录。
function backendAccessLog(ids) {
  const r = spawnSync('docker', [...COMPOSE, 'logs', '--tail', '400', 'backend'],
    { cwd: root, encoding: 'utf8', timeout: 30000 })
  const text = (r.stdout || '') + (r.stderr || '')
  return text.split(/\r?\n/).filter((line) => ids.some((id) => id && line.includes(id)))
}

let token = ''
let classID
async function login() {
  const r = await fetch(`${BASE}/api/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS }),
  })
  if (!r.ok) throw new Error(`登录失败 ${r.status}`)
  const session = await r.json()
  token = session.access_token
  classID = session.user.class_id
  if (!Number.isSafeInteger(classID) || classID <= 0) throw new Error('无有效班级标识')
}
async function api(p, init = {}) {
  const r = await fetch(BASE + p, {
    ...init, headers: { Authorization: `Bearer ${token}`, ...(init.headers || {}) },
  })
  let body = null
  try { body = await r.json() } catch { body = {} }
  // 服务端为每条请求生成并回写 X-Request-ID（spec R10），用它把两次并发请求区分开。
  return { status: r.status, body, raw: JSON.stringify(body), requestID: r.headers.get('x-request-id') || '' }
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
const unique = randomUUID()
async function submit(title, content) {
  const t0 = performance.now()
  const r = await api('/api/materials', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, content }),
  })
  const finished = performance.now()
  if (r.status === 202 && Number.isSafeInteger(r.body.material_id) && r.body.material_id > 0) created.push(r.body.material_id)
  return { ...r, started: t0, finished, elapsed: finished - t0 }
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
async function concurrentUpload() {
  const title = `并发上传-${unique}`
  const content = `单切片材料，唯一标记 ${unique}。`
  // 两个 fetch 均在等待响应前启动；等待所有请求结束后才检查结果或清理。
  const replies = await Promise.allSettled([submit(title, content), submit(title, content)])
  const [a, b] = replies.map((reply, i) => {
    if (reply.status === 'rejected') throw reply.reason
    const { status, body } = reply.value
    say(`- 请求 ${i + 1}：HTTP ${status}；material_id=${body.material_id}；task_id=${body.task_id}`)
    if (status !== 202 || !Number.isSafeInteger(body.material_id) || body.material_id <= 0 ||
        typeof body.task_id !== 'string' || !body.task_id.trim()) {
      throw new Error('并发上传未返回 202 和有效标识')
    }
    return reply.value
  })
  const ids = [a.requestID, b.requestID]
  if (!ids[0] || !ids[1]) throw new Error('响应未带回 X-Request-ID，无法确认是两个独立请求')
  if (ids[0] === ids[1]) throw new Error(`两次请求拿到相同的 request id（${ids[0]}），不是两个独立请求`)
  say(`- 服务端请求标识：${ids[0]} / ${ids[1]}（两条不同，说明服务端收到两次独立请求）`)
  const logged = backendAccessLog(ids)
  if (logged.length) {
    command('docker compose logs --tail 400 backend | grep <两条 request id>')
    output(logged.join('\n'))
  } else {
    say('- 服务端访问日志未采集（容器日志不可达或已滚动）；并发结论以下面的落库计数为准')
  }
  const overlap = Math.min(a.finished, b.finished) - Math.max(a.started, b.started)
  // 两个 fetch 同时发起，在途窗口必然重叠：这个数字只能说明客户端没有串行等待，
  // 不能当作「服务端两个事务同时进行」的直接证据，真正的判据是下面的落库计数。
  say(`- 客户端两请求窗口重叠：${overlap.toFixed(2)} ms（客户端观测值，非服务端并发直证）`)
  if (overlap <= 0) throw new Error('请求未重叠')
  if (a.body.task_id !== b.body.task_id || a.body.material_id !== b.body.material_id) throw new Error('并发上传标识不一致')
  const first = await waitTask(a.body.task_id, 150000)
  if (first?.status !== 'succeeded' || first.material_id !== a.body.material_id) {
    throw new Error(`索引任务未成功关联本次材料：${first?.status ?? '超时'}`)
  }

  // 标题仅由固定前缀和随机 UUID 组成。独立核验实际行数，避免只比较两份回执。
  const rows = mysql(`SELECT COUNT(DISTINCT m.id), COUNT(t.id) FROM materials m
    LEFT JOIN index_tasks t ON t.material_id=m.id AND t.class_id=m.class_id AND t.kind='upload'
    WHERE m.class_id=${classID} AND m.title='${title}'`)
  const chunks = mysql(`SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks
    WHERE material_id=${a.body.material_id} AND class_id=${classID}`)
  const points = qdrantCount(a.body.material_id)
  say(`- 材料/上传任务=${rows}；切片/唯一序号=${chunks}；向量点=${points ?? '未采集'}`)
  if (rows !== '1\t1' || chunks !== '1\t1' || points !== 1) {
    throw new Error('单切片材料应恰有一行材料、一个上传任务、一条切片和一个向量点')
  }
  return a.body.material_id
}

async function criterion3() {
  section('判据三 · 并发提交 / 重复投递只出一份结果')
  const materialID = await concurrentUpload()

  // 对同一份材料再发起一次重索引：成功后 revision+1 会生成新任务，
  // 重复执行全量替换切片与向量，用来验证确定性点 ID 不会让点数翻倍。
  const re = await api(`/api/materials/${materialID}/reindex`, { method: 'POST' })
  const second = re.status === 202 ? await waitTask(re.body.task_id, 150000) : null
  const chunks2 = mysql(`SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=${materialID}`)
  const points2 = qdrantCount(materialID)
  const [total2, distinct2] = chunks2.split(/\s+/).map(Number)
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
  const noDup = second?.status === 'succeeded' && total2 === 1 && distinct2 === 1
  const measuredPoints = points2 !== null
  const noPointDup = measuredPoints && points2 === 1
  // 任一子项未采集就记 SKIP，绝不把「没测到」写成通过。
  const outcome = (!noDup || redelivery === false || (measuredPoints && !noPointDup))
    ? false
    : (!measuredPoints || redelivery === null) ? null : true
  verdict('判据三', outcome,
    `并发入口幂等=true；切片每 seq 一行且不增长=${noDup}；向量点不翻倍=${measuredPoints ? noPointDup : '未采集'}；` +
    `重复投递 attempts≥2=${redelivery === null ? 'SKIP' : redelivery}。` +
    (!measuredPoints ? ' Qdrant 点数未采集（容器内网不可达时记为 SKIP，不算通过）。' : ''))
}

// ---------------------------------------------------------------- 主流程
try {
  await login()
  if (WHICH === 'concurrent') {
    await concurrentUpload()
    verdict('并发上传', true, '两次 202、相同标识，材料/上传任务/切片/向量点各一份')
  }
  if (WHICH === 'all' || WHICH === '1') await criterion1()
  if (WHICH === 'all' || WHICH === '2') await criterion2()
  if (WHICH === 'all' || WHICH === '3') await criterion3()
} catch (e) {
  say(`\n**脚本异常**：${e.stack || e.message}`)
  results['执行'] = 'FAIL'
} finally {
  if (WHICH !== 'concurrent') compose('start', 'worker')
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
  const filename = WHICH === 'concurrent' ? 'concurrent-uploads.md' : 'async-criteria.md'
  writeFileSync(path.join(dir, filename), lines.join('\n') + '\n', 'utf8')
  console.log(`\n归档：reports/${filename}`)
  process.exit(Object.values(results).includes('FAIL') ? 1 : 0)
}
