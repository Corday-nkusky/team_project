// 与后端交互的唯一入口：双令牌保存 / 401 静默续期 / SSE 流式问答。
//
// 凭证载体系「请求头 Bearer token」（design D5）：access token 与 refresh token
// 都存在 localStorage，由本模块负责携带与续期，页面组件不直接接触令牌。

export interface User {
  id: number
  username: string
  role: 'teacher' | 'student'
  class_id: number
  class_name: string
  display_name: string
}

export interface Material {
  id: number
  title: string
  index_status: 'pending' | 'ready' | 'failed'
  index_error?: string
  owner_id: number
  created_at: string
}

export interface Citation {
  index: number
  material_id: number
  material_title: string
  chunk_id: number
  snippet: string
}

interface TokenResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

const ACCESS = 'cc_access'
const REFRESH = 'cc_refresh'
const USER = 'cc_user'

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS)
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH)
}

export function getCurrentUser(): User | null {
  const raw = localStorage.getItem(USER)
  if (!raw) return null
  try {
    return JSON.parse(raw) as User
  } catch {
    return null
  }
}

function saveSession(t: TokenResponse) {
  localStorage.setItem(ACCESS, t.access_token)
  localStorage.setItem(REFRESH, t.refresh_token)
  localStorage.setItem(USER, JSON.stringify(t.user))
}

export function clearSession() {
  localStorage.removeItem(ACCESS)
  localStorage.removeItem(REFRESH)
  localStorage.removeItem(USER)
}

// onUnauthorized 由 App 注入：续期彻底失败时回到登录页。
let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

/** 登录：成功后保存双令牌。 */
export async function login(username: string, password: string): Promise<User> {
  const resp = await fetch('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!resp.ok) {
    const body = await resp.json().catch(() => null)
    throw new Error(body?.error?.message ?? '登录失败')
  }
  const data = (await resp.json()) as TokenResponse
  saveSession(data)
  return data.user
}

/** 登出：作废服务端的 refresh token，并清空本地会话。 */
export async function logout(): Promise<void> {
  const refresh = getRefreshToken()
  try {
    await fetch('/api/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refresh }),
    })
  } finally {
    clearSession()
  }
}

// 模块级互斥：并发请求同时遇到 401 时，只触发一次刷新（design D8）。
let refreshInFlight: Promise<boolean> | null = null

async function refreshToken(): Promise<boolean> {
  const refresh = getRefreshToken()
  if (!refresh) return false
  try {
    const resp = await fetch('/api/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refresh }),
    })
    if (!resp.ok) return false
    saveSession((await resp.json()) as TokenResponse)
    return true
  } catch {
    return false
  }
}

function ensureRefreshed(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = refreshToken().finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

/** 带认证的请求：遇 401 自动续期并重放一次。 */
export async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const send = (token: string | null) => {
    const headers = new Headers(init.headers)
    // 只为字符串体补 JSON 头；FormData 必须让浏览器自己生成 multipart boundary（B01）。
    if (typeof init.body === 'string' && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json')
    }
    if (token) headers.set('Authorization', `Bearer ${token}`)
    return fetch(path, { ...init, headers })
  }

  let resp = await send(getAccessToken())
  if (resp.status === 401) {
    const ok = await ensureRefreshed()
    if (!ok) {
      clearSession()
      onUnauthorized?.()
      return resp
    }
    resp = await send(getAccessToken())
  }
  return resp
}

export async function listMaterials(): Promise<Material[]> {
  const resp = await request('/api/materials')
  if (!resp.ok) throw new Error('获取材料列表失败')
  const data = (await resp.json()) as { materials: Material[] | null }
  return data.materials ?? []
}

export async function uploadText(title: string, content: string): Promise<Material> {
  const resp = await request('/api/materials', {
    method: 'POST',
    body: JSON.stringify({ title, content }),
  })
  if (!resp.ok) {
    const body = await resp.json().catch(() => null)
    throw new Error(body?.error?.message ?? '上传失败')
  }
  return (await resp.json()) as Material
}

export async function uploadFile(title: string, file: File): Promise<Material> {
  const form = new FormData()
  form.append('title', title)
  form.append('file', file)
  // 上传走 multipart，不能带 Content-Type（由浏览器设置 boundary）。
  const resp = await request('/api/materials', { method: 'POST', body: form })
  if (!resp.ok) {
    const body = await resp.json().catch(() => null)
    throw new Error(body?.error?.message ?? '上传失败')
  }
  return (await resp.json()) as Material
}

export async function reindex(id: number): Promise<Material> {
  const resp = await request(`/api/materials/${id}/reindex`, { method: 'POST' })
  if (!resp.ok) throw new Error('重索引失败')
  return (await resp.json()) as Material
}

export interface AskHandlers {
  onMeta?: (citations: Citation[], noEvidence: boolean) => void
  /** 模型在首字前不可用，后端改用材料原文作答（design D3）。 */
  onDegraded?: (reason: string) => void
  onDelta?: (text: string) => void
  onDone?: () => void
  onError?: (message: string) => void
}

/**
 * 流式问答：用 fetch 读取 ReadableStream 并手工分帧解析 SSE。
 *
 * 注意：不复用 EventSource——它只支持 GET，而本接口是带鉴权头的 POST。
 * 返回的 AbortController 供调用方在组件卸载 / 用户中断时取消。
 */
export function streamAsk(question: string, handlers: AskHandlers): AbortController {
  const controller = new AbortController()

  ;(async () => {
    let resp: Response
    try {
      resp = await request('/api/ask', {
        method: 'POST',
        body: JSON.stringify({ question }),
        signal: controller.signal,
      })
    } catch (e) {
      if ((e as Error).name !== 'AbortError') handlers.onError?.('请求失败')
      return
    }
    if (!resp.ok) {
      const body = await resp.json().catch(() => null)
      handlers.onError?.(body?.error?.message ?? '问答失败')
      return
    }
    if (!resp.body) {
      handlers.onError?.('当前浏览器不支持流式响应')
      return
    }

    const reader = resp.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''
    let eventName = ''
    let dataLines: string[] = []
    // settled 表示本次流已经以 done 或 error 收尾；
    // 连接在两者之前结束，一律按中断处理，不能把半截回答当完整（design D3）。
    let settled = false

    const fail = (message: string) => {
      if (settled) return
      settled = true
      handlers.onError?.(message)
    }

    const dispatch = () => {
      if (!eventName || dataLines.length === 0) {
        eventName = ''
        dataLines = []
        return
      }
      const data = dataLines.join('\n')
      try {
        if (eventName === 'meta') {
          const m = JSON.parse(data) as { citations: Citation[]; no_evidence: boolean }
          handlers.onMeta?.(m.citations ?? [], m.no_evidence)
        } else if (eventName === 'degraded') {
          const d = JSON.parse(data) as { reason?: string }
          handlers.onDegraded?.(d.reason ?? 'upstream_error')
        } else if (eventName === 'delta') {
          const d = JSON.parse(data) as { text: string }
          if (d.text) handlers.onDelta?.(d.text)
        } else if (eventName === 'error') {
          const d = JSON.parse(data) as { code?: string; message?: string }
          fail(d.message || '生成中断，请重试')
        } else if (eventName === 'done') {
          if (!settled) {
            settled = true
            handlers.onDone?.()
          }
        }
      } catch {
        /* 忽略无法解析的帧 */
      }
      eventName = ''
      dataLines = []
    }

    try {
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        let idx: number
        while ((idx = buffer.indexOf('\n')) >= 0) {
          const line = buffer.slice(0, idx).replace(/\r$/, '')
          buffer = buffer.slice(idx + 1)
          if (line === '') {
            dispatch()
          } else if (line.startsWith('event:')) {
            eventName = line.slice(6).trim()
          } else if (line.startsWith('data:')) {
            dataLines.push(line.slice(5).replace(/^ /, ''))
          }
        }
      }
      dispatch()
      // 流结束却没有 done/error：上游被截断或代理提前关闭。
      fail('连接中断，请重试')
    } catch (e) {
      if ((e as Error).name !== 'AbortError') fail('连接中断，请重试')
    }
  })()

  return controller
}
