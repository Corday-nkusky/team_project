import { useEffect, useRef, useState } from 'react'
import { getMaterial, listMaterials, reindex, uploadFile, uploadText, type Material, type MaterialDetail, type User } from '../api'
import Icon from '../components/Icon'

export default function MaterialsPage({ user }: { user: User }) {
  const [items, setItems] = useState<Material[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [reindexing, setReindexing] = useState<number | null>(null)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)
  const [readerId, setReaderId] = useState<number | null>(null)
  const [reader, setReader] = useState<MaterialDetail | null>(null)
  const [readerLoading, setReaderLoading] = useState(false)
  const [readerError, setReaderError] = useState('')
  const readerReq = useRef(0)
  const readerRef = useRef<HTMLDivElement>(null)
  const readerCloseRef = useRef<HTMLButtonElement>(null)
  const readerTrigger = useRef<HTMLElement | null>(null)

  const isTeacher = user.role === 'teacher'
  const titleLen = [...title].length
  const readyCount = items.filter((item) => item.index_status === 'ready').length
  const failedCount = items.filter((item) => item.index_status === 'failed').length
  const readerItem = items.find((item) => item.id === readerId)

  const refresh = async () => {
    setLoading(true)
    try {
      setItems(await listMaterials())
      setLoadError('')
    } catch (e) {
      setLoadError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  useEffect(() => {
    if (readerId === null) return
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    readerCloseRef.current?.focus()
    return () => {
      document.body.style.overflow = previous
    }
  }, [readerId])

  const closeReader = () => {
    readerReq.current += 1
    setReaderId(null)
    setReader(null)
    setReaderError('')
    setReaderLoading(false)
    requestAnimationFrame(() => readerTrigger.current?.focus())
  }

  const openMaterial = async (id: number, trigger?: HTMLElement) => {
    if (trigger) readerTrigger.current = trigger
    const req = ++readerReq.current
    setReaderId(id)
    setReader(null)
    setReaderError('')
    setReaderLoading(true)
    try {
      const detail = await getMaterial(id)
      if (readerReq.current !== req) return
      setReader(detail)
    } catch (e) {
      if (readerReq.current !== req) return
      setReaderError((e as Error).message)
    } finally {
      if (readerReq.current === req) setReaderLoading(false)
    }
  }

  const handleReaderKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      closeReader()
      return
    }
    if (e.key !== 'Tab' || !readerRef.current) return
    const focusables = [...readerRef.current.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    )]
    if (focusables.length === 0) return
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }

  const handleText = async () => {
    setBusy(true)
    setError('')
    setNotice('')
    try {
      const m = await uploadText(title, content)
      setNotice(m.index_status === 'ready' ? `已上传并建立索引：${m.title}` : `已上传，但索引失败：${m.index_error}`)
      setTitle('')
      setContent('')
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
    setNotice('')
    try {
      const m = await uploadFile(title || file.name, file)
      setNotice(m.index_status === 'ready' ? `已上传并建立索引：${m.title}` : `已上传，但索引失败：${m.index_error}`)
      setTitle('')
      if (fileRef.current) fileRef.current.value = ''
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
    try {
      const m = await reindex(id)
      setNotice(m.index_status === 'ready' ? '重索引成功' : `重索引仍失败：${m.index_error}`)
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setReindexing(null)
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
            <button className="ghost" onClick={refresh}><Icon name="refresh" size={16} />重试</button>
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
                        onClick={(e) => void openMaterial(m.id, e.currentTarget)}
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

      {readerId !== null && (
        <div
          className="reader-overlay"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) closeReader()
          }}
        >
          <div
            ref={readerRef}
            className="reader-panel"
            role="dialog"
            aria-modal="true"
            aria-labelledby="reader-title"
            onKeyDown={handleReaderKeyDown}
          >
            <header className="reader-header">
              <div className="reader-heading">
                <span className="eyebrow">MATERIAL</span>
                <h2 id="reader-title">{reader?.title ?? readerItem?.title ?? '材料正文'}</h2>
                <div className="reader-meta">
                  {reader && (
                    <span className={`status ${reader.index_status}`}><i />{statusText(reader.index_status)}</span>
                  )}
                  {reader && <span><Icon name="clock" size={14} />{formatDate(reader.created_at)}</span>}
                  <span><Icon name="shield" size={14} />仅本班可见</span>
                </div>
              </div>
              <button
                ref={readerCloseRef}
                type="button"
                className="reader-close"
                onClick={closeReader}
                aria-label="关闭阅读面板"
              >
                <Icon name="close" size={18} />
              </button>
            </header>

            <div className="reader-body">
              {readerLoading && (
                <div className="reader-loading" role="status">
                  <span className="spinner dark" />
                  <p>正在读取材料原文…</p>
                </div>
              )}

              {!readerLoading && readerError && (
                <div className="reader-error" role="alert">
                  <span className="state-icon"><Icon name="alert" size={24} /></span>
                  <div>
                    <strong>暂时无法读取这篇材料</strong>
                    <p>{readerError}</p>
                  </div>
                  <button
                    className="ghost"
                    onClick={() => {
                      if (readerId !== null) void openMaterial(readerId)
                    }}
                  >
                    <Icon name="refresh" size={16} />重试读取
                  </button>
                </div>
              )}

              {!readerLoading && !readerError && reader && (
                reader.content.trim() ? (
                  <pre className="material-content">{reader.content}</pre>
                ) : (
                  <div className="reader-empty">
                    <span className="state-icon"><Icon name="book" size={26} /></span>
                    <h3>材料正文为空</h3>
                    <p>这份材料没有可显示的文本，请联系上传教师确认原文。</p>
                  </div>
                )
              )}
            </div>

            <footer className="reader-footer">
              <span><kbd>Esc</kbd> 关闭</span>
              <span>原文按纯文本展示，不执行其中的格式或脚本。</span>
            </footer>
          </div>
        </div>
      )}
    </div>
  )
}
