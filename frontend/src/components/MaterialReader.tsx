import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from 'react'
import { getMaterial, type MaterialDetail } from '../api'
import Icon from './Icon'

/**
 * 材料全文阅读面板：材料列表、回答引用、历史问答三处共用同一份实现。
 *
 * 与旧版材料页阅读器行为一致（四态、Esc 关闭、焦点归还、Tab 循环），
 * 额外支持传入命中切片：在整篇原文里定位、高亮并滚动到可见位置。
 */
export interface MaterialReaderOptions {
  title?: string
  snippet?: string
  trigger?: HTMLElement | null
}

export interface MaterialReaderHandle {
  open: (materialId: number, options?: MaterialReaderOptions) => void
  close: () => void
}

interface ReaderRequest {
  id: number
  title: string
  snippet: string
  trigger: HTMLElement | null
}

const statusText = (s: MaterialDetail['index_status']) =>
  s === 'ready' ? '可检索' : s === 'failed' ? '索引失败' : '处理中'

const formatDate = (value: string) => new Date(value).toLocaleString('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false,
})

const MaterialReader = forwardRef<MaterialReaderHandle>(function MaterialReader(_props, ref) {
  const [req, setReq] = useState<ReaderRequest | null>(null)
  const [detail, setDetail] = useState<MaterialDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const reqSeq = useRef(0)
  const panelRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const hitRef = useRef<HTMLElement>(null)

  const load = useCallback(async (target: ReaderRequest) => {
    const seq = ++reqSeq.current
    setDetail(null)
    setError('')
    setLoading(true)
    try {
      const d = await getMaterial(target.id)
      if (reqSeq.current !== seq) return
      setDetail(d)
    } catch (e) {
      if (reqSeq.current !== seq) return
      setError((e as Error).message)
    } finally {
      if (reqSeq.current === seq) setLoading(false)
    }
  }, [])

  const open = useCallback((id: number, options: MaterialReaderOptions = {}) => {
    const target: ReaderRequest = {
      id,
      title: options.title ?? '材料正文',
      snippet: options.snippet ?? '',
      trigger: options.trigger ?? null,
    }
    setReq(target)
    void load(target)
  }, [load])

  const close = useCallback(() => {
    reqSeq.current += 1
    setReq(null)
    setDetail(null)
    setError('')
    setLoading(false)
    requestAnimationFrame(() => req?.trigger?.focus())
  }, [req])

  useImperativeHandle(ref, () => ({ open, close }), [open, close])

  useEffect(() => {
    if (!req) return
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    closeRef.current?.focus()
    return () => { document.body.style.overflow = previous }
  }, [req])

  // 命中切片必须真的存在于正文里才高亮；找不到时照常显示全文并明确提示。
  const hitIndex = detail && req?.snippet ? detail.content.indexOf(req.snippet) : -1
  useEffect(() => {
    if (hitIndex < 0) return
    const timer = window.setTimeout(() => hitRef.current?.scrollIntoView({ block: 'center' }), 0)
    return () => window.clearTimeout(timer)
  }, [hitIndex, req, detail])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      close()
      return
    }
    if (e.key !== 'Tab' || !panelRef.current) return
    const focusables = [...panelRef.current.querySelectorAll<HTMLElement>(
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

  if (!req) return null

  return (
    <div
      className="reader-overlay"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) close()
      }}
    >
      <div
        ref={panelRef}
        className="reader-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="reader-title"
        onKeyDown={handleKeyDown}
      >
        <header className="reader-header">
          <div className="reader-heading">
            <span className="eyebrow">MATERIAL</span>
            <h2 id="reader-title">{detail?.title ?? req.title}</h2>
            <div className="reader-meta">
              {detail && (
                <span className={`status ${detail.index_status}`}><i />{statusText(detail.index_status)}</span>
              )}
              {detail && <span><Icon name="clock" size={14} />{formatDate(detail.created_at)}</span>}
              {req.snippet && hitIndex >= 0 && <span><Icon name="check" size={14} />已定位到引用片段</span>}
              <span><Icon name="shield" size={14} />仅本班可见</span>
            </div>
          </div>
          <button
            ref={closeRef}
            type="button"
            className="reader-close"
            onClick={close}
            aria-label="关闭阅读面板"
          >
            <Icon name="close" size={18} />
          </button>
        </header>

        <div className="reader-body">
          {loading && (
            <div className="reader-loading" role="status">
              <span className="spinner dark" />
              <p>正在读取材料原文…</p>
            </div>
          )}

          {!loading && error && (
            <div className="reader-error" role="alert">
              <span className="state-icon"><Icon name="alert" size={24} /></span>
              <div>
                <strong>暂时无法读取这篇材料</strong>
                <p>{error}</p>
              </div>
              <button className="ghost" onClick={() => void load(req)}>
                <Icon name="refresh" size={16} />重试读取
              </button>
            </div>
          )}

          {!loading && !error && detail && (
            detail.content.trim() ? (
              <>
                {req.snippet && hitIndex < 0 && (
                  <p className="reader-locate-note" role="status">未能定位到引用片段，以下仍为整篇材料原文。</p>
                )}
                <pre className="material-content">
                  {hitIndex >= 0 ? (
                    <>
                      {detail.content.slice(0, hitIndex)}
                      <mark ref={hitRef} className="material-hit">
                        {detail.content.slice(hitIndex, hitIndex + req.snippet.length)}
                      </mark>
                      {detail.content.slice(hitIndex + req.snippet.length)}
                    </>
                  ) : detail.content}
                </pre>
              </>
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
  )
})

export default MaterialReader
