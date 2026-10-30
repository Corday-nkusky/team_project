#!/usr/bin/env node
// 前端端到端验证：用真实 Chrome（CDP 驱动）走一遍「登录 → 上传（文本/文件）→ 流式问答 → 引用」，
// 并覆盖加载/空/错误态、停止与中断标记、窄屏排版。
//
//   node scripts/e2e-ui.mjs                       # 默认打 http://127.0.0.1:8080
//   BASE=http://127.0.0.1:8080 node scripts/e2e-ui.mjs
//
// 退出码即判定：全部通过 0，任一失败 1。

import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
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
  askQuestion: (q) => {
    window.__t.setVal(document.querySelector('#q'), q);
    window.__t.byText('button', '提问').click();
  },
  installFetchMock: () => {
    if (window.__origFetch) return true;
    window.__origFetch = window.fetch.bind(window);
    window.__matMode = 'normal';
    window.__askMode = 'normal';
    window.__matResolve = null;
    window.fetch = (input, init) => {
      const url = typeof input === 'string' ? input : input.url;
      const method = ((init && init.method) || 'GET').toUpperCase();
      if (typeof url === 'string' && url.includes('/api/materials') && method === 'GET') {
        if (window.__matMode === 'delay') {
          return new Promise((resolve, reject) => {
            window.__matResolve = () => window.__origFetch(input, init).then(resolve, reject);
          });
        }
        if (window.__matMode === 'fail') {
          return Promise.resolve(new Response(
            JSON.stringify({ error: { code: 'internal', message: '模拟故障' } }),
            { status: 500, headers: { 'Content-Type': 'application/json' } },
          ));
        }
        if (window.__matMode === 'empty') {
          return Promise.resolve(new Response(
            JSON.stringify({ materials: [] }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ));
        }
      }
      if (typeof url === 'string' && url.includes('/api/ask') && method === 'POST' && window.__askMode !== 'normal') {
        const mode = window.__askMode;
        return Promise.resolve(new Response(new ReadableStream({
          start(controller) {
            const enc = new TextEncoder();
            const push = (s) => controller.enqueue(enc.encode(s));
            push('event: meta\\ndata: {"citations":[],"no_evidence":false}\\n\\n');
            push('event: delta\\ndata: {"text":"慢流开始。"}\\n\\n');
            if (mode === 'slow') {
              const signal = init && init.signal;
              if (signal) {
                signal.addEventListener('abort', () => {
                  try { controller.error(new DOMException('Aborted', 'AbortError')) } catch (e) {}
                });
              }
            } else if (mode === 'cut') {
              controller.close();
            } else if (mode === 'error') {
              push('event: error\\ndata: {"code":"stream_incomplete","message":"生成中断，请重试"}\\n\\n');
              controller.close();
            } else if (mode === 'degraded') {
              push('event: degraded\\ndata: {"reason":"upstream_error"}\\n\\n');
              push('event: delta\\ndata: {"text":"降级回答。"}\\n\\n');
              push('event: done\\ndata: {"conversation_id":null}\\n\\n');
              controller.close();
            }
          }
        }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } }));
      }
      return window.__origFetch(input, init);
    };
    return true;
  },
  setMatMode: (m) => { window.__matMode = m },
  setAskMode: (m) => { window.__askMode = m },
  resolveMat: () => {
    if (!window.__matResolve) return false;
    const f = window.__matResolve;
    window.__matResolve = null;
    f();
    return true;
  },
  restoreFetch: () => {
    if (window.__origFetch) window.fetch = window.__origFetch;
    window.__origFetch = null;
    window.__matMode = 'normal';
    window.__askMode = 'normal';
  },
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
    const { send, evaluate, on, close } = await connect(wsUrl)

    try {
      await send('Runtime.enable')
      await send('Page.enable')
      await send('DOM.enable')
      await send('Network.enable')

      // 记录文件上传请求的 Content-Type，确认浏览器保留了 multipart boundary（B01）。
      let multipartCT = ''
      on('Network.requestWillBeSent', (params) => {
        if (params.request.method === 'POST' && params.request.url.endsWith('/api/materials')) {
          const headers = params.request.headers || {}
          const ct = headers['Content-Type'] || headers['content-type']
          if (ct) multipartCT = ct
        }
      })

      console.log(`== CampusClaw 前端端到端 ==  base=${BASE}\n`)

      await send('Page.navigate', { url: BASE })
      await sleep(600)
      await evaluate(HELPERS)

      const unique = `界面验证${Date.now() % 1000000}`
      const materialTitle = `界面验证材料`
      const marker = `尾注${unique}`
      // 语料要能真正回答问题，模型才有依据可引用（也能顺带验证引用标记 R7.1）；
      // 正文超过 300 字并在末尾放独有尾注，用来验证引用回看不是截断摘要（R7.5）。
      const materialBody =
        `${materialTitle}（唯一标识 ${unique}）：CampusClaw 使用说明——教师需要在「教学材料」页上传讲义，上传后系统会自动建立索引；` +
        `学生在「提问问答」页提交问题，系统只在本班材料内检索，并给出带引用的回答。` +
        '本段用于验证引用回看不会被截断。'.repeat(14) +
        `（${marker}）`

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
      ok('T3b 标题输入限制为 255 字并显示计数', await evaluate(`(() => {
        const t = document.querySelector('#t');
        t.focus();
        t.select();
        // 用真实编辑命令输入 300 字，验证浏览器按 maxLength 截断到 255。
        document.execCommand('insertText', false, '长'.repeat(300));
        return t.maxLength === 255 && [...t.value].length === 255 &&
          /当前 255 字/.test(document.body.innerText);
      })()`))
      await evaluate(`window.__t.setVal(document.querySelector('#t'), '')`)

      // T4 流式问答 + 引用
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      await evaluate(`window.__t.waitFor(() => document.querySelector('#q'))`)
      // 带上本轮独有词，保证新上传的材料排在 topK 内（历史运行会累积同主题材料）。
      await evaluate(`window.__t.askQuestion('教师应该在哪里上传讲义？' + ${JSON.stringify(unique)})`)
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
      ok(
        'T5b 引用片段为完整切片，包含 300 字之后的尾注',
        await evaluate(`window.__t.waitFor(() => {
           const el = [...document.querySelectorAll('.cite-item')]
             .find(e => e.innerText.includes(${JSON.stringify(unique)}));
           return el && el.innerText.includes(${JSON.stringify(marker)});
         }, 5000).then(() => true).catch(() => false)`),
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
      await evaluate(`window.__t.askQuestion('请解释一个本班材料完全没有涉及的主题：' + Date.now())`)
      const noEvidence = await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('资料中未找到相关内容')).then(() => true).catch(() => false)`,
      )
      ok('T6 材料中没有依据时给出固定拒答文案', noEvidence)
      await evaluate(
        `window.__t.waitFor(() => !window.__t.text().includes('流式生成中')).then(() => true).catch(() => false)`,
      )

      // T9 真实文件选择上传（multipart boundary 不得被覆盖）
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      await evaluate(`window.__t.waitFor(() => document.querySelector('#f'))`)
      const uploadName = `e2e-file-${unique}.md`
      const uploadPath = join(userDataDir, uploadName)
      writeFileSync(uploadPath, `# 文件上传验证\n\n这是通过真实文件选择框上传的材料，独有词 ${unique}F。\n`)
      multipartCT = ''
      await evaluate(`window.__t.setVal(document.querySelector('#t'), '')`)
      const doc = await send('DOM.getDocument', { depth: -1 })
      const fileNode = await send('DOM.querySelector', { nodeId: doc.root.nodeId, selector: '#f' })
      await send('DOM.setFileInputFiles', { files: [uploadPath], nodeId: fileNode.nodeId })
      const fileUploaded = await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes(${JSON.stringify(uploadName)}) && window.__t.text().includes('可检索'))
           .then(() => true).catch(() => false)`,
      )
      ok('T9 通过真实文件选择框上传 .md 成功', fileUploaded)
      ok('T9b 上传请求 Content-Type 保留 multipart boundary', /^multipart\/form-data;\s*boundary=/.test(multipartCT), multipartCT)

      // T10 材料列表加载 / 错误 / 空 / 重试
      await evaluate(`window.__t.installFetchMock()`)
      await evaluate(`window.__t.setMatMode('delay')`)
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      const loadingShown = await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('加载中')).then(() => true).catch(() => false)`,
      )
      ok('T10a 加载中显示占位', loadingShown)
      ok('T10b 加载中不显示「还没有材料」空态', await evaluate(`!window.__t.text().includes('还没有材料')`))
      await evaluate(`window.__t.resolveMat()`)
      ok('T10c 加载完成后显示列表', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes(${JSON.stringify(uploadName)})).then(() => true).catch(() => false)`,
      ))

      await evaluate(`window.__t.setMatMode('fail')`)
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      ok('T10d 请求失败时显示错误与「重试」', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('材料列表加载失败') && !!window.__t.byText('button', '重试'))
           .then(() => true).catch(() => false)`,
      ))
      await evaluate(`window.__t.setMatMode('normal')`)
      await evaluate(`window.__t.byText('button', '重试').click()`)
      ok('T10e 点击重试后列表恢复', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes(${JSON.stringify(uploadName)}) && !window.__t.text().includes('材料列表加载失败'))
           .then(() => true).catch(() => false)`,
      ))

      await evaluate(`window.__t.setMatMode('empty')`)
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      ok('T10f 空列表给出教师上传引导', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('还没有材料，请在下方上传第一份讲义。'))
           .then(() => true).catch(() => false)`,
      ))
      await evaluate(`window.__t.setMatMode('normal')`)
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      await evaluate(`window.__t.restoreFetch()`)

      // T11 停止 / 中断 / 降级状态
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      await evaluate(`window.__t.waitFor(() => document.querySelector('#q'))`)
      await evaluate(`window.__t.installFetchMock()`)
      await evaluate(`window.__t.setAskMode('slow')`)
      await evaluate(`window.__t.askQuestion('停止验证：慢流')`)
      await evaluate(`window.__t.waitFor(() => document.querySelector('.answer') && document.querySelector('.answer').innerText.includes('慢流开始')).then(() => true).catch(() => false)`)
      await evaluate(`window.__t.byText('button', '停止').click()`)
      ok('T11a 点击停止后显示「已停止生成」且不再显示生成中', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('已停止生成')).then(() => true).catch(() => false)
         && !window.__t.text().includes('流式生成中')`,
      ))

      await evaluate(`window.__t.setAskMode('cut')`)
      await evaluate(`window.__t.askQuestion('中断验证：无声截断')`)
      ok('T11b 流未经 done 结束时标记「生成中断」', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('生成中断，以上为不完整内容') && window.__t.text().includes('连接中断'))
           .then(() => true).catch(() => false)`,
      ))

      await evaluate(`window.__t.setAskMode('error')`)
      await evaluate(`window.__t.askQuestion('中断验证：服务端 error 事件')`)
      ok('T11c 服务端 error 事件显示可读文案', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('生成中断，请重试')).then(() => true).catch(() => false)`,
      ))

      await evaluate(`window.__t.setAskMode('degraded')`)
      await evaluate(`window.__t.askQuestion('降级验证')`)
      ok('T11d degraded 事件显示「模型暂不可用」', await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes('模型暂不可用，已改用材料原文作答'))
           .then(() => true).catch(() => false)`,
      ))
      await evaluate(`window.__t.restoreFetch()`)

      // T12 窄屏排版与 255 字标题
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      await evaluate(`window.__t.waitFor(() => document.querySelector('#t'))`)
      const longTitle = '长标题' + '甲'.repeat(252)
      await evaluate(`(() => {
        window.__t.setVal(document.querySelector('#t'), ${JSON.stringify(longTitle)});
        window.__t.setVal(document.querySelector('#c'), '标题排版验证正文，独有词 ${unique}LAYOUT。');
      })()`)
      await evaluate(`window.__t.byText('button', '上传文本').click()`)
      const longUploaded = await evaluate(
        `window.__t.waitFor(() => [...document.querySelectorAll('.mat-title')].some(e => e.getAttribute('title') === ${JSON.stringify(longTitle)}))
           .then(() => true).catch(() => false)`,
      )
      ok('T12a 255 字标题可上传并出现在列表', longUploaded)

      await send('Emulation.setDeviceMetricsOverride', { width: 524, height: 900, deviceScaleFactor: 1, mobile: false })
      await sleep(300)
      const layout = await evaluate(`(() => {
        const th = [...document.querySelectorAll('th')].find(e => e.textContent.trim() === '索引状态');
        const row = [...document.querySelectorAll('tr')].find(tr => {
          const td = tr.querySelector('.mat-title');
          return td && td.getAttribute('title') === ${JSON.stringify(longTitle)};
        });
        const badge = row && row.querySelector('.status');
        if (!th || !row || !badge) return null;
        return {
          thWhite: getComputedStyle(th).whiteSpace,
          thH: th.getBoundingClientRect().height,
          badgeW: badge.getBoundingClientRect().width,
          badgeH: badge.getBoundingClientRect().height,
          rowH: row.getBoundingClientRect().height,
          overflow: document.documentElement.scrollWidth - window.innerWidth,
        };
      })()`)
      console.log(`  · 524px 实测: ${JSON.stringify(layout)}`)
      ok('T12b 524px 下「索引状态」表头保持单行', layout && layout.thWhite === 'nowrap' && layout.thH <= 40, JSON.stringify(layout))
      ok('T12c 状态徽标保持单行（宽 > 高）', layout && layout.badgeH <= 40 && layout.badgeW > layout.badgeH, JSON.stringify(layout))
      ok('T12d 255 字标题行高 ≤ 120px', layout && layout.rowH <= 120, JSON.stringify(layout))
      ok('T12e 窄屏无横向溢出', layout && layout.overflow <= 1, JSON.stringify(layout))
      await send('Emulation.clearDeviceMetricsOverride')
      await sleep(200)

      // T7 access 失效后静默续期
      await evaluate(`window.__t.byText('button', '提问问答').click()`)
      ok('T7 本地存在 access / refresh 双令牌', await evaluate(`!!localStorage.getItem('cc_access') && !!localStorage.getItem('cc_refresh')`))
      await evaluate(`localStorage.setItem('cc_access', 'tampered.token.value')`)
      await evaluate(`window.__t.byText('button', '教学材料').click()`)
      const recovered = await evaluate(
        `window.__t.waitFor(() => window.__t.text().includes(${JSON.stringify(uploadName)})).then(() => true).catch(() => false)`,
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
    const listeners = new Map()

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

      const on = (method, cb) => {
        const arr = listeners.get(method) || []
        arr.push(cb)
        listeners.set(method, arr)
      }

      resolve({ send, evaluate, on, close: () => ws.close() })
    })
    ws.addEventListener('message', (ev) => {
      const m = JSON.parse(ev.data)
      if (m.id) {
        const cb = pending.get(m.id)
        if (cb) {
          pending.delete(m.id)
          cb(m)
        }
        return
      }
      const cbs = listeners.get(m.method)
      if (cbs) cbs.forEach((cb) => cb(m.params))
    })
    ws.addEventListener('error', reject)
  })
}

main().catch((e) => {
  console.error('端到端脚本异常：', e.message)
  process.exit(1)
})
