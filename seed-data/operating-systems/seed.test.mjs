import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { spawn } from 'node:child_process'
import { mkdtempSync, readFileSync, writeFileSync, copyFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { loadLectures } from './seed.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
test('OS1 六份正文校验通过，英文讲义已中文化', () => {
  const docs = loadLectures()
  assert.equal(docs.length, 6)
  for (const doc of docs.slice(3, 5)) {
    assert.match(doc.bags[0], /进程|页表/)
    assert.doesNotMatch(doc.bags[0], /[A-Za-z][A-Za-z ]{45,}/)
  }
})

test('OS1 缺失正文与校验和错误均被拒绝', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'os-data-test-'))
  try {
    const rows = JSON.parse(readFileSync(path.join(here, 'corpus.json'), 'utf8'))
    copyFileSync(path.join(here, 'corpus.json'), path.join(dir, 'corpus.json'))
    assert.throws(() => loadLectures(dir), /ENOENT/)
    for (const row of rows) copyFileSync(path.join(here, row.file), path.join(dir, row.file))
    writeFileSync(path.join(dir, rows[0].file), 'unexpected content')
    assert.throws(() => loadLectures(dir), /校验和/)
  } finally { rmSync(dir, { recursive: true, force: true }) }
})

async function fixture(t, { role = 'teacher', classID = 1, conflict = false, failed = false } = {}) {
  const docs = loadLectures()
  const materials = conflict ? [{ id: 1, title: docs[0].materialTitle, content: 'old', index_status: 'ready' }] : []
  let uploads = 0
  const server = createServer(async (req, res) => {
    const reply = (body, status = 200) => { res.writeHead(status, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(body)) }
    let body = ''
    for await (const chunk of req) body += chunk
    if (req.url === '/api/login') return reply({ access_token: 'fixture-access', user: { role, class_id: classID } })
    if (req.headers.authorization !== 'Bearer fixture-access') return reply({}, 401)
    if (req.url === '/api/materials' && req.method === 'GET') return reply({ materials })
    if (req.url === '/api/materials' && req.method === 'POST') {
      const data = JSON.parse(body); const id = materials.length + 1
      materials.push({ id, ...data, index_status: failed ? 'failed' : 'ready' }); uploads++
      return reply({ material_id: id, task_id: String(id), status: 'pending', status_url: `/api/tasks/${id}` }, 202)
    }
    if (req.url.startsWith('/api/tasks/')) return reply({ status: failed ? 'failed' : 'succeeded' })
    if (/^\/api\/materials\/\d+$/.test(req.url)) return reply(materials.find(m => m.id === Number(req.url.split('/').at(-1))))
    reply({}, 404)
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(() => new Promise(resolve => server.close(resolve)))
  const run = () => new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [path.join(here, 'seed.mjs')], {
      env: { ...process.env, BASE: `http://127.0.0.1:${server.address().port}`, SEED_USER: 'fixture', SEED_PASS: 'fixture' },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let output = ''
    child.stdout.on('data', b => { output += b }); child.stderr.on('data', b => { output += b })
    child.on('error', reject); child.on('exit', code => resolve({ code, output }))
  })
  return { run, count: () => uploads }
}

test('OS2 上传索引成功，再次导入无重复', async t => {
  const f = await fixture(t)
  const first = await f.run(); assert.equal(first.code, 0, first.output); assert.equal(f.count(), 6)
  const second = await f.run(); assert.equal(second.code, 0, second.output); assert.equal(f.count(), 6)
  assert.match(second.output, /跳过 6/)
})
for (const [label, settings] of [['学生', { role: 'student' }], ['跨班教师', { classID: 2 }]]) {
  test(`OS2 ${label}在上传前被拒绝`, async t => {
    const f = await fixture(t, settings); const result = await f.run()
    assert.notEqual(result.code, 0); assert.equal(f.count(), 0); assert.match(result.output, /目标班级教师/)
  })
}
test('OS2 同标题正文冲突拒绝覆盖', async t => {
  const f = await fixture(t, { conflict: true }); const result = await f.run()
  assert.notEqual(result.code, 0); assert.equal(f.count(), 0); assert.match(result.output, /正文冲突/)
})
test('OS2 索引失败返回非零', async t => {
  const f = await fixture(t, { failed: true }); const result = await f.run()
  assert.notEqual(result.code, 0); assert.match(result.output, /失败/)
})
