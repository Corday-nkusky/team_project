import { useEffect, useRef, useState } from 'react'
import { listMaterials, reindex, uploadFile, uploadText, waitForTask,
  type Material, type User } from '../api'
import Icon from '../components/Icon'
import MaterialReader, { type MaterialReaderHandle } from '../components/MaterialReader'

export default function MaterialsPage({ user }: { user: User }) {
  const [items, setItems] = useState<Material[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [reindexing, setReindexing] = useState<number | null>(null)
  const [bulkBusy, setBulkBusy] = useState(false)
  const [bulkProgress, setBulkProgress] = useState('')
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)
  const readerRef = useRef<MaterialReaderHandle>(null)

  const isTeacher = user.role === 'teacher'
  const titleLen = [...title].length
  const readyCount = items.filter((item) => item.index_status === 'ready').length
  const failedCount = items.filter((item) => item.index_status === 'failed').length
  // 未索引 = 尚未 ready（pending / failed）；批量按钮只处理这一类。
  const pendingItems = items.filter((item) => item.index_status !== 'ready')

  const refresh = async (silent = false) => {
    if (!silent) setLoading(true)
    try {
      setItems(await listMaterials())
      setLoadError('')
    } catch (e) {
      setLoadError((e as Error).message)
    } finally {
      if (!silent) setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  // 有材料仍在 pending 时静默刷新列表，页面刷新后也能看到任务完成。
  useEffect(() => {
    if (loading || loadError || !items.some((item) => item.index_status === 'pending')) return
    const timer = window.setTimeout(() => { void refresh(true) }, 1500)
    return () => window.clearTimeout(timer)
  }, [items, loading, loadError])

  // 提交只是拿到号码牌，建立索引由后台 worker 完成；这里轮询到终态再刷新列表。
  const settleNotice = (state: Awaited<ReturnType<typeof waitForTask>>, label: string) =>
    state.status === 'succeeded'
      ? `${label}完成，材料已可检索`
      : `${label}失败：${state.error || `任务状态 ${state.status}`}`

  const handleText = async () => {
    setBusy(true)
    setError('')
    setNotice('已提交，正在后台建立索引…')
    try {
      const task = await uploadText(title, content)
      const state = await waitForTask(task.task_id)
      setNotice(settleNotice(state, '索引'))
      if (state.status === 'succeeded') {
        setTitle('')
        setContent('')
      }
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const handleFile = async (file: File) => {
    setBusy(true)
    setError('')
    setNotice('已提交，正在后台建立索引…')
    try {
      const task = await uploadFile(title || file.name, file)
      const state = await waitForTask(task.task_id)
      setNotice(settleNotice(state, '索引'))
      if (state.status === 'succeeded') {
        setTitle('')
        if (fileRef.current) fileRef.current.value = ''
      }
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const handleReindex = async (id: number) => {
    setReindexing(id)
    setError('')
    setNotice('已提交重索引任务…')
    try {
      const task = await reindex(id)
      const state = await waitForTask(task.task_id)
      setNotice(state.status === 'succeeded' ? '重索引成功' : `重索引仍失败：${state.error || `任务状态 ${state.status}`}`)
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setReindexing(null)
    }
  }

  // 一键索引：把所有非 ready 的材料逐批交给后台 worker，等每批到终态再继续。
  // 并发度与 worker 默认并发一致，避免瞬间堆一堆领不走的 pending 任务。
  const BULK_CONCURRENCY = 4
  const handleIndexAll = async () => {
    const targets = [...pendingItems]
    if (bulkBusy || targets.length === 0) return
    setBulkBusy(true)
    setError('')
    setNotice('')
    let ok = 0, failed = 0, done = 0, throttled = false
    setBulkProgress(`已提交 0/${targets.length}，后台建立索引中…`)
    try {
      for (let start = 0; start < targets.length && !throttled; start += BULK_CONCURRENCY) {
        const batch = targets.slice(start, start + BULK_CONCURRENCY)
        const results = await Promise.allSettled(batch.map(async (m) => {
          const task = await reindex(m.id)
          const state = await waitForTask(task.task_id)
          if (state.status !== 'succeeded') throw new Error(state.error || `任务状态 ${state.status}`)
        }))
        for (const r of results) {
          done++
          if (r.status === 'fulfilled') ok++
          else {
            failed++
            const reason = r.reason as Error | undefined
            const msg = reason?.message ?? String(reason ?? '')
            if (/429|额度|quota|rate limit|limit/i.test(msg)) throttled = true
          }
        }
        setBulkProgress(`已完成 ${done}/${targets.length}（成功 ${ok}，失败 ${failed}）…`)
      }
      setNotice(throttled
        ? `网关限流，已停止批量索引：成功 ${ok}，失败 ${failed}，稍后可重试`
        : failed > 0
          ? `批量索引完成：成功 ${ok}，失败 ${failed}`
          : `批量索引完成，${ok} 份材料已可检索`)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      await refresh(true)
      setBulkProgress('')
      setBulkBusy(false)
    }
  }

  const statusText = (s: Material['index_status']) =>
    s === 'ready' ? '可检索' : s === 'failed' ? '索引失败' : '处理中'

  const formatDate = (value: string) => new Date(value).toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false,
  })

  return (
    <div className={`materials-layout ${isTeacher ? '' : 'readonly'}`}>
      <section className="materials-panel">
        <div className="panel-heading">
          <div>
            <span className="eyebrow">MATERIALS</span>
            <h2>本班教学材料</h2>
            <p>{user.class_name} · 所有内容严格限定在班级内</p>
          </div>
          <div className="material-count"><strong>{items.length}</strong><span>份材料</span></div>
        </div>

        {!loading && !loadError && items.length > 0 && (
          <div className="material-summary">
            <span><i className="summary-dot ready" />{readyCount} 份可检索</span>
            {failedCount > 0 && <span><i className="summary-dot failed" />{failedCount} 份索引失败</span>}
            {isTeacher && (
              <button
                type="button"
                className="ghost bulk-index"
                onClick={() => void handleIndexAll()}
                disabled={bulkBusy || busy || pendingItems.length === 0}
                title="为所有索引失败或仍在处理中的材料重建索引"
              >
                <Icon name="refresh" size={14} />
                {bulkBusy ? '索引中…' : `一键索引未索引（${pendingItems.length}）`}
              </button>
            )}
            {bulkProgress && <span className="bulk-progress" role="status">{bulkProgress}</span>}
            <span className="summary-note"><Icon name="shield" size={14} />仅本班可见</span>
          </div>
        )}

        {loading && (
          <div className="material-loading" role="status">
            <div className="skeleton-row"><span /><span /><span /></div>
            <div className="skeleton-row"><span /><span /><span /></div>
            <p><span className="spinner dark" />加载中…</p>
          </div>
        )}

        {!loading && loadError && (
          <div className="state-block error-state">
            <span className="state-icon"><Icon name="alert" size={24} /></span>
            <div><strong>暂时无法读取材料</strong><div className="error">材料列表加载失败：{loadError}</div></div>
            <button className="ghost" onClick={() => void refresh()}><Icon name="refresh" size={16} />重试</button>
          </div>
        )}

        {!loading && !loadError && items.length === 0 && (
          <div className="empty-state">
            <span className="state-icon"><Icon name="book" size={28} /></span>
            <h3>{isTeacher ? '从第一份讲义开始' : '等待教师上传材料'}</h3>
            <p>{isTeacher ? '还没有材料，请在下方上传第一份讲义。' : '本班还没有材料，请联系教师上传。'}</p>
          </div>
        )}

        {!loading && !loadError && items.length > 0 && (
          <div className="material-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>标题</th>
                  <th className="col-status">索引状态</th>
                  <th className="col-time">上传时间</th>
                  <th className="col-action"><span className="hidden">操作</span></th>
                </tr>
              </thead>
              <tbody>
                {items.map((m) => (
                  <tr key={m.id}>
                    <td className="mat-title" title={m.title}>
                      <button
                        type="button"
                        className="mat-title-button"
                        onClick={(e) => readerRef.current?.open(m.id, { title: m.title, trigger: e.currentTarget })}
                        aria-label={`阅读材料：${m.title}`}
                      >
                        <span>{m.title}</span>
                        <Icon name="arrow" size={13} />
                      </button>
                    </td>
                    <td className="col-status">
                      <span className={`status ${m.index_status}`}><i />{statusText(m.index_status)}</span>
                    </td>
                    <td className="col-time"><Icon name="clock" size={14} />{formatDate(m.created_at)}</td>
                    <td className="col-action">
                      {isTeacher && m.index_status === 'failed' && (
                        <button className="ghost retry-index" onClick={() => handleReindex(m.id)} disabled={reindexing === m.id}>
                          <Icon name="refresh" size={14} />{reindexing === m.id ? '索引中…' : '重试索引'}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {isTeacher && (
        <aside className="upload-panel">
          <div className="upload-heading">
            <span className="section-icon"><Icon name="upload" size={21} /></span>
            <div><h2>上传材料</h2><p>上传后自动切片并建立索引</p></div>
          </div>

          <div className="upload-rule"><Icon name="shield" size={16} />支持纯文本 / Markdown / PDF，单个文件不超过 32 MiB。</div>

          <div className="field">
            <label htmlFor="t">标题（最多 255 字，当前 {titleLen} 字）</label>
            <input
              id="t"
              value={title}
              maxLength={255}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="例如：第一节讲义"
              disabled={busy}
            />
          </div>
          <div className="field">
            <label htmlFor="c">正文（纯文本 / Markdown）</label>
            <textarea
              id="c"
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder="粘贴讲义正文，系统只会将其作为本班检索资料…"
              disabled={busy}
            />
          </div>
          <button className="primary upload-text-button" onClick={handleText} disabled={busy || !content.trim()}>
            {busy ? <span className="spinner" /> : <Icon name="upload" size={17} />}
            {busy ? '上传中…' : '上传文本'}
          </button>

          <div className="upload-divider"><span>或选择文件</span></div>
          <label className={`file-drop ${busy ? 'disabled' : ''}`} htmlFor="f">
            <span className="file-drop-icon"><Icon name="file" size={24} /></span>
            <span><strong>选择讲义文件</strong><small>.txt / .md / .markdown / .pdf</small></span>
            <span className="file-action">浏览</span>
          </label>
          <input
            id="f"
            className="file-input"
            type="file"
            ref={fileRef}
            accept=".txt,.md,.markdown,.pdf"
            disabled={busy}
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (f) void handleFile(f)
            }}
          />

          {notice && <p className={`notice ${notice.includes('失败') ? 'warning' : ''}`} role="status"><Icon name={notice.includes('失败') ? 'alert' : 'check'} size={17} />{notice}</p>}
          {error && <div className="error" role="alert"><Icon name="alert" size={17} />{error}</div>}
        </aside>
      )}

      <MaterialReader ref={readerRef} />
    </div>
  )
}
