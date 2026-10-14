#!/usr/bin/env node
// 前端端到端验证：用真实 Chrome（CDP 驱动）走一遍「登录 → 上传 → 流式问答 → 点开引用」。
//
//   node scripts/e2e-ui.mjs                       # 默认打 http://127.0.0.1:8080
//   BASE=http://127.0.0.1:8080 node scripts/e2e-ui.mjs
//
// 退出码即判定：全部通过 0，任一失败 1。

import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const BASE = process.env.BASE || 'http://127.0.0.1:8080'
const PORT = Number(process.env.CDP_PORT || 9333)
const CHROME =
  process.env.CHROME ||
  'C:/Program Files/Google/Chrome/Application/chrome.exe'

let passed = 0
let failed = 0
const results = []

function ok(name, cond, detail = '') {
  if (cond) {
    passed++
    console.log(`  \x1b[32m✓\x1b[0m ${name}`)
  } else {
    failed++
    console.log(`  \x1b[31m✗\x1b[0m ${name}  ${detail}`)
  }
  results.push({ name, ok: !!cond, detail })
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// 注入页面的辅助工具（React 受控组件必须走原生 setter + input 事件）。
const HELPERS = `
window.__t = {
  byText: (sel, text) => {
    const all = [...document.querySelectorAll(sel)];
    return all.find(e => e.textContent.trim() === text)
        || all.find(e => e.textContent.trim().includes(text));
  },
  setVal: (el, v) => {
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, v);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  },
  waitFor: (fn, timeout = 25000) => new Promise((res, rej) => {
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

async function main() {
  const userDataDir = mkdtempSync(join(tmpdir(), 'cc-e2e-'))
  const chrome = spawn(
    CHROME,
    [
      '--headless=new',
      '--disable-gpu',
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-extensions',
      `--remote-debugging-port=${PORT}`,
      `--user-data-dir=${userDataDir}`,
      'about:blank',
    ],
    { stdio: 'ignore' },
  )

  try {
    const wsUrl = await waitForTarget()
    const { send, evaluate, close } = await connect(wsUrl)

    try {
      await send('Runtime.enable')
      await send('Page.enable')

      console.log(`== CampusClaw 前端端到端 ==  base=${BASE}\n`)

      await send('Page.navigate', { url: BASE })
      await sleep(600)
      await evaluate(HELPERS)

      const unique = `界面验证${Date.now() % 1000000}`
      const materialTitle = `界面验证材料`
      // 语料要能真正回答问题，模型才有依据可引用（也能顺带验证引用标记 R7.1）。
      const materialBody =
        `${materialTitle}（唯一标识 ${unique}）：CampusClaw 使用说明——教师需要在「教学材料」页上传讲义，上传后系统会自动建立索引；` +
        `学生在「提问问答」页提交问题，系统只在本班材料内检索，并给出带引用的回答。`

      // T1 登录页
      await evaluate(`window.__t.waitFor(() => document.querySelector('#u'))`)
      ok('T1 打开首页显示登录表单', await evaluate(`!!document.querySelector('#u') && !!document.querySelector('#p')`))

      // T2 登录（教师）
      await evaluate(`(() => {
        window.__t.setVal(document.querySelector('#u'), 'teacher1');
        window.__t.setVal(document.querySelector('#p'), 'teacher123');
        window.__t.byText('button', '登录').click();
      })()`)
      const loggedIn = await evaluate(
        `window.__t.waitFor(() => document.body.innerText.includes('退出登录')).then(() => true).catch(() => false)`,
      )
      ok('T2 正确凭据登录后进入主界面', loggedIn)
      ok('T2b 顶栏显示角色与班级', await evaluate(`document.body.innerText.includes('教师') && document.body.innerText.includes('班')`))

      // T3 上传材料
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      await evaluate(`window.__t.waitFor(() => document.querySelector('#t'))`)
      await evaluate(`(() => {
        window.__t.setVal(document.querySelector('#t'), ${JSON.stringify(unique)});
        window.__t.setVal(document.querySelector('#c'), ${JSON.stringify(materialBody)});
      })()`)
      await evaluate(`window.__t.byText('button', '上传文本').click()`)
      const uploaded = await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes(${JSON.stringify(unique)}) && window.__t.text().includes('可检索'))
           .then(() => true).catch(() => false)`,
      )
      ok('T3 教师上传材料后列表出现且状态「可检索」', uploaded)

      // T4 流式问答 + 引用
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      await evaluate(`window.__t.waitFor(() => document.querySelector('#q'))`)
      await evaluate(`(() => {
        window.__t.setVal(document.querySelector('#q'), '教师应该在哪里上传讲义？');
        window.__t.byText('button', '提问').click();
      })()`)
      const streamed = await evaluate(
        `window.__t.waitFor(() => document.querySelector('.answer') && document.querySelector('.answer').innerText.trim().length > 0)
           .then(() => true).catch(() => false)`,
      )
      ok('T4 提问后回答以流式渲染出现', streamed)
      // 引用标记可能分片到达（"[" 与 "1" 是两个增量），需等待其出现。
      ok(
        'T4b 回答中含可点击的 [n] 引用标记',
        await evaluate(`window.__t.waitFor(() => !!document.querySelector('.cite')).then(() => true).catch(() => false)`),
      )
      ok(
        'T4c 引用清单列出本次上传的材料',
        await evaluate(
          `window.__t.waitFor(() => [...document.querySelectorAll('.cite-item')].some(e => e.innerText.includes(${JSON.stringify(unique)})))
             .then(() => true).catch(() => false)`,
        ),
      )

      // T5 点开引用
      await evaluate(`document.querySelector('.cite').click()`)
      ok(
        'T5 点击 [n] 可展开对应材料原文片段',
        await evaluate(
          `window.__t.waitFor(() => {
             const el = document.querySelector('.cite-item');
             return el && el.innerText.includes('《');
           }, 5000).then(() => true).catch(() => false)`,
        ),
      )

      // T6 未命中不编造
      // 先等上一轮流结束：流式期间提交按钮文案为「生成中…」，此时 byText('button','提问')
      // 会退化为包含匹配、误点顶部页签「提问问答」，导致本轮请求根本没发出。
      await evaluate(
        `window.__t.waitFor(() => {
           const b = window.__t.byText('button', '提问');
           return b && b.textContent.trim() === '提问';
         }).then(() => true).catch(() => false)`,
      )
      await evaluate(`(() => {
        window.__t.setVal(document.querySelector('#q'), '请解释一个本班材料完全没有涉及的主题：' + Date.now());
        window.__t.byText('button', '提问').click();
      })()`)
      const noEvidence = await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('资料中未找到相关内容')).then(() => true).catch(() => false)`,
      )
      ok('T6 材料中没有依据时给出固定拒答文案', noEvidence)
      // 等本轮流结束，避免下一轮切换页签时把请求中断。
      await evaluate(
        `window.__t.waitFor(() => !window.__t.text().includes('流式生成中')).then(() => true).catch(() => false)`,
      )

      // T7 access 失效后静默续期
      ok('T7 本地存在 access / refresh 双令牌', await evaluate(`!!localStorage.getItem('cc_access') && !!localStorage.getItem('cc_refresh')`))
      await evaluate(`localStorage.setItem('cc_access', 'tampered.token.value')`)
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      const recovered = await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes(${JSON.stringify(unique)})).then(() => true).catch(() => false)`,
      )
      ok('T7b access 被篡改后仍能加载数据（自动续期并重放）', recovered)
      ok('T7c 续期后本地令牌已更新', await evaluate(`localStorage.getItem('cc_access') !== 'tampered.token.value'`))

      // T8 登出
      await evaluate(`window.__t.byText('button', '退出登录').click()`)
      const loggedOut = await evaluate(
        `window.__t.waitFor(() => document.querySelector('#u')).then(() => true).catch(() => false)`,
      )
      ok('T8 登出后回到登录页并清空本地会话', loggedOut && (await evaluate(`!localStorage.getItem('cc_refresh')`)))

      console.log(`\n== 合计: ${passed} 通过, ${failed} 失败 ==`)
    } finally {
      close()
    }
  } finally {
    chrome.kill()
    await sleep(300)
    rmSync(userDataDir, { recursive: true, force: true })
  }

  process.exit(failed > 0 ? 1 : 0)
}

async function waitForTarget() {
  for (let i = 0; i < 100; i++) {
    try {
      const list = await fetch(`http://127.0.0.1:${PORT}/json/list`).then((r) => r.json())
      const target = list.find((t) => t.type === 'page')
      if (target?.webSocketDebuggerUrl) return target.webSocketDebuggerUrl
    } catch {
      /* 还没起来 */
    }
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
      const send = (method, params = {}) =>
        new Promise((res, rej) => {
          const mid = ++id
          pending.set(mid, (m) => (m.error ? rej(new Error(`${method}: ${m.error.message}`)) : res(m.result)))
          ws.send(JSON.stringify({ id: mid, method, params }))
        })

      const evaluate = async (expression) => {
        const r = await send('Runtime.evaluate', {
          expression,
          awaitPromise: true,
          returnByValue: true,
        })
        if (r.exceptionDetails) {
          throw new Error(r.exceptionDetails.exception?.description || '页面脚本异常')
        }
        return r.result.value
      }

      resolve({ send, evaluate, close: () => ws.close() })
    })
    ws.addEventListener('message', (ev) => {
      const m = JSON.parse(ev.data)
      const cb = pending.get(m.id)
      if (cb) {
        pending.delete(m.id)
        cb(m)
      }
    })
    ws.addEventListener('error', reject)
  })
}

main().catch((e) => {
  console.error('端到端脚本异常：', e.message)
  process.exit(1)
})
