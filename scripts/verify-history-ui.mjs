#!/usr/bin/env node
// 定向浏览器验收：历史问答面板 + 引用「阅读全文」+ 材料页共用阅读面板。
//
//   node scripts/verify-history-ui.mjs
//
// 与 e2e-ui.mjs 的区别：这里**不上传任何材料、不产生任何测试文本**，
// 用真实存量会话与真实材料详情验证界面；问答那一步用页面内 fetch 桩，
// 引用数据取自真实历史记录，因此也不新增会话记录。退出码即判定。

import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const BASE = process.env.BASE || 'http://127.0.0.1:8080'
const PORT = Number(process.env.CDP_PORT || 9335)
const CHROME = process.env.CHROME || (() => {
  if (process.platform === 'win32') return 'C:/Program Files/Google/Chrome/Application/chrome.exe'
  if (process.platform === 'darwin') return '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
  return 'google-chrome'
})()

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
let passed = 0, failed = 0
const ok = (name, cond, detail = '') => {
  if (cond) { passed++; console.log(`  \x1b[32m✓\x1b[0m ${name}`) }
  else { failed++; console.log(`  \x1b[31m✗\x1b[0m ${name}  ${detail}`) }
}

const HELPERS = `
window.__v = {
  setVal: (el, v) => {
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, v);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  },
  byText: (sel, text) => [...document.querySelectorAll(sel)].find(e => e.textContent.trim() === text)
      || [...document.querySelectorAll(sel)].find(e => e.textContent.trim().includes(text)),
  waitFor: (fn, timeout = 20000) => new Promise((res, rej) => {
    const t0 = Date.now();
    const tick = () => {
      let v = null;
      try { v = fn() } catch (e) { v = null }
      if (v) return res(true);
      if (Date.now() - t0 > timeout) return rej(new Error('waitFor 超时'));
      setTimeout(tick, 100);
    };
    tick();
  }),
  text: () => document.body.innerText,
};
true;
`

async function waitForTarget() {
  for (let i = 0; i < 100; i++) {
    try {
      const list = await fetch(`http://127.0.0.1:${PORT}/json/list`).then((r) => r.json())
      const target = list.find((t) => t.type === 'page')
      if (target?.webSocketDebuggerUrl) return target.webSocketDebuggerUrl
    } catch { /* Chrome 还没起来 */ }
    await sleep(200)
  }
  throw new Error('无法连接到 Chrome 调试端口')
}

function connect(wsUrl) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(wsUrl)
    let id = 0
    const pending = new Map()
    ws.addEventListener('open', () => {
      const send = (method, params = {}) => new Promise((res, rej) => {
        const mid = ++id
        pending.set(mid, (m) => (m.error ? rej(new Error(`${method}: ${m.error.message}`)) : res(m.result)))
        ws.send(JSON.stringify({ id: mid, method, params }))
      })
      const evaluate = async (expression) => {
        const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
        if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || '页面脚本异常')
        return r.result.value
      }
      resolve({ send, evaluate, close: () => ws.close() })
    })
    ws.addEventListener('message', (ev) => {
      const m = JSON.parse(ev.data)
      if (m.id) {
        const cb = pending.get(m.id)
        if (cb) { pending.delete(m.id); cb(m) }
      }
    })
    ws.addEventListener('error', reject)
  })
}

async function pressEscape(send) {
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 })
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27, nativeVirtualKeyCode: 27 })
}

async function main() {
  const userDataDir = mkdtempSync(join(tmpdir(), 'cc-history-'))
  const chrome = spawn(CHROME, [
    '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
    '--disable-extensions', `--remote-debugging-port=${PORT}`, `--user-data-dir=${userDataDir}`, 'about:blank',
  ], { stdio: 'ignore' })
  const launchError = new Promise((_, reject) => {
    chrome.once('error', (err) => reject(new Error(`无法启动 Chrome（${CHROME}）：${err.message}`)))
  })

  try {
    const wsUrl = await Promise.race([waitForTarget(), launchError])
    const { send, evaluate, close } = await connect(wsUrl)
    try {
      await send('Runtime.enable')
      await send('Page.enable')
      await send('Page.navigate', { url: BASE })
      await sleep(800)
      await evaluate(HELPERS)

      console.log(`== 历史问答 / 引用全文 定向验收 ==  base=${BASE}\n`)

      await evaluate(`(() => {
        window.__v.setVal(document.querySelector('#u'), 'teacher1');
        window.__v.setVal(document.querySelector('#p'), 'teacher123');
        window.__v.byText('button', '登录').click();
        return true;
      })()`)
      const loggedIn = await evaluate(`window.__v.waitFor(() => window.__v.text().includes('退出登录')).then(() => true).catch(() => false)`)
      ok('U0 登录进入工作台', loggedIn)

      // U1 历史面板：真实存量会话，最多 20 条
      const h = await evaluate(`window.__v.waitFor(() => document.querySelector('.history-card') && document.querySelectorAll('.history-item').length > 0)
        .then(() => ({ card: !!document.querySelector('.history-card'), count: document.querySelectorAll('.history-item').length,
                       error: !!document.querySelector('.history-error') })).catch(() => null)`)
      ok('U1 历史面板列出真实记录且无错误态', !!h && h.card && h.count > 0 && !h.error, JSON.stringify(h))
      console.log(`     历史条数 = ${h?.count ?? 0}`)

      // U2 展开一条带引用的历史，完整回答与引用清单可见
      const expanded = await evaluate(`(async () => {
        const heads = [...document.querySelectorAll('.history-item-head')];
        for (const head of heads.slice(0, 8)) {
          head.click();
          await window.__v.waitFor(() => document.querySelector('.history-answer'), 3000).catch(() => null);
          if (document.querySelector('.history-cite')) {
            return { answer: document.querySelector('.history-answer').innerText.length,
                     cites: document.querySelectorAll('.history-cite').length,
                     snippet: document.querySelector('.history-cite .cite-snippet')?.innerText.length || 0 };
          }
        }
        return null;
      })()`)
      ok('U2 展开历史可看完整回答与当时的引用', !!expanded && expanded.answer > 0 && expanded.cites > 0, JSON.stringify(expanded))

      // U3 历史引用「阅读全文」打开整篇原文并高亮命中切片
      const full = await evaluate(`(async () => {
        const btn = document.querySelector('.history-cite .cite-fulltext');
        if (!btn) return null;
        window.__openedFrom = btn;
        btn.click();
        await window.__v.waitFor(() => document.querySelector('.reader-panel .material-content'), 20000).catch(() => null);
        const content = document.querySelector('.reader-panel .material-content');
        const hit = document.querySelector('.reader-panel .material-hit');
        return { panel: !!document.querySelector('.reader-panel'),
                 chars: content?.innerText.length || 0,
                 hit: hit?.innerText.length || 0,
                 note: !!document.querySelector('.reader-locate-note') };
      })()`)
      ok('U3 阅读全文显示整篇材料且命中切片已高亮', !!full && full.panel && full.chars > (full.hit || 0) && full.hit > 0, JSON.stringify(full))
      console.log(`     全文 ${full?.chars ?? 0} 字 / 高亮切片 ${full?.hit ?? 0} 字`)

      // U4 Esc 关闭并把焦点还给触发按钮
      await pressEscape(send)
      const afterEsc = await evaluate(`(async () => {
        await window.__v.waitFor(() => !document.querySelector('.reader-panel'), 5000).catch(() => null);
        await window.__v.waitFor(() => document.activeElement === window.__openedFrom, 3000).catch(() => null);
        return { closed: !document.querySelector('.reader-panel'),
                 focusBack: document.activeElement === window.__openedFrom,
                 active: document.activeElement?.className || '' };
      })()`)
      ok('U4 Esc 关闭阅读面板且焦点归还', afterEsc.closed && afterEsc.focusBack, JSON.stringify(afterEsc))

      // U5 材料页仍用同一套阅读面板（重构未回归）
      await evaluate(`(() => { window.__v.byText('button', '教学材料').click(); return true })()`)
      const mat = await evaluate(`(async () => {
        await window.__v.waitFor(() => document.querySelector('.mat-title-button'), 20000);
        document.querySelector('.mat-title-button').click();
        await window.__v.waitFor(() => document.querySelector('.reader-panel .material-content'), 20000).catch(() => null);
        return { panel: !!document.querySelector('.reader-panel'),
                 chars: document.querySelector('.reader-panel .material-content')?.innerText.length || 0 };
      })()`)
      ok('U5 材料页阅读面板仍可打开全文', !!mat && mat.panel && mat.chars > 0, JSON.stringify(mat))
      await pressEscape(send)

      // U6 即时回答的引用也能打开全文：用真实历史引用做页面内 /api/ask 桩，不新增数据
      await evaluate(`(() => { window.__v.byText('button', '提问问答').click(); return true })()`)
      const cite = await evaluate(`(async () => {
        const token = localStorage.getItem('cc_access');
        const r = await fetch('/api/conversations?limit=8', { headers: { Authorization: 'Bearer ' + token } });
        const d = await r.json();
        const hit = (d.items || []).find(x => (x.citations || []).length > 0);
        window.__cite = hit?.citations?.[0] || null;
        return window.__cite;
      })()`)
      ok('U6a 取到一条真实引用作为问答桩数据', !!cite && !!cite.material_id, JSON.stringify(cite && { id: cite.material_id, snippet: cite.snippet?.length }))

      await evaluate(`(() => {
        window.__orig = window.fetch;
        window.fetch = (input, init) => {
          if (String(input).includes('/api/ask') && init && init.method === 'POST') {
            const s = 'event: meta\\ndata: ' + JSON.stringify({ citations: [window.__cite], no_evidence: false }) + '\\n\\n'
              + 'event: delta\\ndata: ' + JSON.stringify({ text: '定向验证回答 [1]' }) + '\\n\\n'
              + 'event: done\\ndata: {"conversation_id":null}\\n\\n';
            return Promise.resolve(new Response(new ReadableStream({ start(c) { c.enqueue(new TextEncoder().encode(s)); c.close(); } }),
              { status: 200, headers: { 'Content-Type': 'text/event-stream' } }));
          }
          return window.__orig(input, init);
        };
        window.__v.setVal(document.querySelector('#q'), '定向验证：引用全文入口');
        window.__v.byText('button', '提问').click();
        return true;
      })()`)
      const instant = await evaluate(`(async () => {
        await window.__v.waitFor(() => document.querySelector('.citations-panel .cite-fulltext'), 20000).catch(() => null);
        const btn = document.querySelector('.citations-panel .cite-fulltext');
        if (!btn) return null;
        window.__openedFrom = btn;
        btn.click();
        await window.__v.waitFor(() => document.querySelector('.reader-panel .material-content'), 20000).catch(() => null);
        const content = document.querySelector('.reader-panel .material-content');
        const hit = document.querySelector('.reader-panel .material-hit');
        window.fetch = window.__orig;
        return { button: true, chars: content?.innerText.length || 0, hit: hit?.innerText.length || 0 };
      })()`)
      ok('U6b 即时回答引用同样打开整篇全文并高亮', !!instant && instant.chars > 0 && instant.hit > 0, JSON.stringify(instant))
      await pressEscape(send)

      console.log(`\n合计：${passed} 通过 / ${failed} 失败`)
      close()
    } catch (e) {
      console.error('验收脚本异常：', e.message)
      failed++
      close()
    }
  } finally {
    chrome.kill()
    try { rmSync(userDataDir, { recursive: true, force: true }) } catch { /* 忽略临时目录清理失败 */ }
  }
  process.exit(failed === 0 ? 0 : 1)
}

main().catch((e) => { console.error(e); process.exit(1) })
