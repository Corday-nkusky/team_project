import { useEffect, useRef, useState } from 'react'
import { streamAsk, type Citation } from '../api'

export default function AskPage() {
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState('')
  const [citations, setCitations] = useState<Citation[]>([])
  const [noEvidence, setNoEvidence] = useState(false)
  const [streaming, setStreaming] = useState(false)
  const [error, setError] = useState('')
  const [openCite, setOpenCite] = useState<number | null>(null)

  // 用 ref 持有 AbortController：组件卸载 / 新一轮提问时中断上一条流（R6.3）。
  const controllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => controllerRef.current?.abort()
  }, [])

  const ask = () => {
    const q = question.trim()
    if (!q || streaming) return

    controllerRef.current?.abort()
    setAnswer('')
    setCitations([])
    setNoEvidence(false)
    setError('')
    setOpenCite(null)
    setStreaming(true)

    controllerRef.current = streamAsk(q, {
      onMeta: (cs, ne) => {
        setCitations(cs)
        setNoEvidence(ne)
      },
      onDelta: (t) => setAnswer((prev) => prev + t),
      onDone: () => setStreaming(false),
      onError: (msg) => {
        setError(msg)
        setStreaming(false)
      },
    })
  }

  const stop = () => {
    controllerRef.current?.abort()
    setStreaming(false)
  }

  /** 把回答里的 [n] 渲染成可点击的引用标记（R7.1）。 */
  const renderAnswer = (text: string) => {
    const parts = text.split(/(\[\d+\])/g)
    return parts.map((p, i) => {
      const m = /^\[(\d+)\]$/.exec(p)
      if (!m) return <span key={i}>{p}</span>
      const idx = Number(m[1])
      const cite = citations.find((c) => c.index === idx)
      if (!cite) return <span key={i}>{p}</span>
      return (
        <span key={i} className="cite" onClick={() => setOpenCite(openCite === idx ? null : idx)}>
          [{idx}]
        </span>
      )
    })
  }

  const opened = citations.find((c) => c.index === openCite)

  return (
    <>
      <div className="card">
        <h2>向本班材料提问</h2>
        <p className="hint">
          回答只依据你所在班级已索引的材料；材料中没有依据时会明确告知，不会编造。
        </p>
        <label htmlFor="q">问题</label>
        <textarea
          id="q"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="例如：这份讲义里提到的核心概念是什么？"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) ask()
          }}
        />
        <button className="primary" onClick={ask} disabled={streaming || !question.trim()}>
          {streaming ? '生成中…' : '提问'}
        </button>
        {streaming && (
          <button className="ghost" style={{ marginLeft: 8 }} onClick={stop}>
            停止
          </button>
        )}
        {error && <div className="error">{error}</div>}
      </div>

      {(answer || noEvidence) && (
        <div className="card">
          <h2>回答 {streaming && <span className="hint">（流式生成中…）</span>}</h2>
          <div className="answer">{renderAnswer(answer)}</div>

          {opened && (
            <div className="list">
              <div className="cite-item">
                <strong>
                  引用 [{opened.index}] · 《{opened.material_title}》
                </strong>
                {opened.snippet}
              </div>
            </div>
          )}

          {citations.length > 0 && (
            <>
              <h2 style={{ marginTop: 20 }}>引用来源（{citations.length}）</h2>
              <div className="list">
                {citations.map((c) => (
                  <div
                    key={c.chunk_id}
                    className="cite-item"
                    onClick={() => setOpenCite(openCite === c.index ? null : c.index)}
                    style={{ cursor: 'pointer' }}
                  >
                    <strong>
                      [{c.index}] 《{c.material_title}》
                    </strong>
                    {c.snippet}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
      )}
    </>
  )
}
