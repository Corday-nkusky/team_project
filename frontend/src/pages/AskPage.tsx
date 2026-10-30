import { useEffect, useRef, useState } from 'react'
import { streamAsk, type Citation } from '../api'

type Phase = 'idle' | 'streaming' | 'done' | 'error' | 'stopped'

export default function AskPage() {
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState('')
  const [citations, setCitations] = useState<Citation[]>([])
  const [noEvidence, setNoEvidence] = useState(false)
  const [phase, setPhase] = useState<Phase>('idle')
  const [degraded, setDegraded] = useState(false)
  const [error, setError] = useState('')
  const [openCite, setOpenCite] = useState<number | null>(null)

  // 用 ref 持有 AbortController：组件卸载 / 新一轮提问时中断上一条流（R6.3）。
  const controllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => controllerRef.current?.abort()
  }, [])

  const streaming = phase === 'streaming'

  const ask = () => {
    const q = question.trim()
    if (!q || streaming) return

    controllerRef.current?.abort()
    setAnswer('')
    setCitations([])
    setNoEvidence(false)
    setDegraded(false)
    setError('')
    setOpenCite(null)
    setPhase('streaming')

    controllerRef.current = streamAsk(q, {
      onMeta: (cs, ne) => {
        setCitations(cs)
        setNoEvidence(ne)
      },
      onDegraded: () => setDegraded(true),
      onDelta: (t) => setAnswer((prev) => prev + t),
      onDone: () => setPhase('done'),
      onError: (msg) => {
        setError(msg)
        setPhase('error')
      },
    })
  }

  const stop = () => {
    controllerRef.current?.abort()
    setPhase('stopped')
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

  // 流结束但不算「正常完成」时，明确标出状态，避免半截回答看起来像完整回答（B13）。
  const stateLabel = () => {
    if (phase === 'streaming') return <span className="hint">（流式生成中…）</span>
    if (phase === 'stopped') return <span className="state-tag stopped">已停止生成，以上为不完整内容</span>
    if (phase === 'error') return <span className="state-tag failed">生成中断，以上为不完整内容</span>
    if (degraded) return <span className="state-tag degraded">模型暂不可用，已改用材料原文作答</span>
    if (phase === 'done' && noEvidence) return <span className="state-tag degraded">材料中无依据，未生成回答</span>
    return null
  }

  return (
    <>
      <div className="card">
        <h2>向本班材料提问</h2>
        <p className="hint">
          回答只依据你所在班级的材料；材料里没有依据时会明确告知，不会编造。
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
          <h2>
            回答 {stateLabel()}
          </h2>
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
