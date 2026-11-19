import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

const here = path.dirname(fileURLToPath(import.meta.url))

export function loadLectures(directory = here) {
  const entries = JSON.parse(readFileSync(path.join(directory, 'corpus.json'), 'utf8'))
  if (!Array.isArray(entries) || entries.length !== 6) throw new Error('必须提供六份讲义')
  const titles = new Set()
  return entries.map(({ title, file, sha256 }) => {
    if (typeof title !== 'string' || !title.trim() || titles.has(title)) throw new Error('讲义标题缺失或重复')
    titles.add(title)
    if (typeof file !== 'string' || !/^[a-z0-9-]+\.md$/.test(file)) throw new Error('讲义文件名不合法')
    const bytes = readFileSync(path.join(directory, file))
    const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/\r\n/g, '\n')
    if (createHash('sha256').update(text).digest('hex') !== sha256) throw new Error(`校验和不匹配：${file}`)
    if (text.trim().length < 500 || /整理说明|获取日期|参考答案|课堂练习|课后练习|复习题与练习|CC BY|https?:\/\//.test(text)) {
      throw new Error(`讲义正文不符合要求：${file}`)
    }
    return { domain: '操作系统', pageTitle: title, materialTitle: title, bags: [text] }
  })
}

export function main(args = process.argv.slice(2)) {
  const lectures = loadLectures()
  if (args.includes('--check')) {
    console.log(`检查通过：${lectures.length} 份中文讲义，正文与校验和有效`)
    return 0
  }
  if (args.length) throw new Error('仅支持 --check；正常导入不带参数')
  const temp = mkdtempSync(path.join(tmpdir(), 'campusclaw-os-'))
  try {
    const corpus = path.join(temp, 'corpus.json')
    writeFileSync(corpus, JSON.stringify(lectures), 'utf8')
    const result = spawnSync(process.execPath, [path.join(here, '../biology/seed.mjs')], {
      stdio: 'inherit',
      env: {
        ...process.env,
        USER: process.env.SEED_USER || 'teacher1',
        PASS: process.env.SEED_PASS || 'teacher123',
        EXPECTED_CLASS_ID: '1', VERIFY_EXISTING_CONTENT: '1',
        CORPUS_PATH: corpus, MANIFEST_PATH: path.join(temp, 'manifest.json'),
      },
    })
    if (result.error) throw result.error
    return result.status ?? 1
  } finally {
    rmSync(temp, { recursive: true, force: true })
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.exitCode = main() }
  catch (error) { console.error(error.message); process.exitCode = 1 }
}
