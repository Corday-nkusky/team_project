import { useEffect, useRef, useState } from 'react'
import { streamAsk, type Citation } from '../api'
import Icon from '../components/Icon'

type Phase = 'idle' | 'streaming' | 'done' | 'error' | 'stopped'

const SUGGESTIONS = [
  '酶为什么能加快化学反应？',
  '有丝分裂和减数分裂的主要区别是什么？',
  '孟德尔分离定律的核心内容是什么？',
]

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

  const toggleCitation = (index: number) => {
    setOpenCite((current) => current === index ? null : index)
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
        <button
          type="button"
          key={i}
          className="cite"
          onClick={() => toggleCitation(idx)}
          aria-label={`查看引用 ${idx}`}
          aria-pressed={openCite === idx}
        >
          [{idx}]
        </button>
      )
    })
  }

  // 流结束但不算「正常完成」时，明确标出状态，避免半截回答看起来像完整回答（B13）。
  const stateLabel = () => {
    if (phase === 'streaming') return <span className="stream-state" role="status"><span className="live-dot" />流式生成中…</span>
    if (phase === 'stopped') return <span className="state-tag stopped">已停止生成，以上为不完整内容</span>
    if (phase === 'error') return <span className="state-tag failed">生成中断，以上为不完整内容</span>
    if (degraded) return <span className="state-tag degraded">模型暂不可用，已改用材料原文作答</span>
    if (phase === 'done' && noEvidence) return <span className="state-tag degraded">材料中无依据，未生成回答</span>
    if (phase === 'done') return <span className="state-tag complete"><Icon name="check" size={14} />回答完成</span>
    return null
  }

  return (
    <div className="ask-layout">
      <section className="composer-card">
        <div className="composer-heading">
          <span className="section-icon"><Icon name="spark" size={22} /></span>
          <div>
            <h2>向本班材料提问</h2>
            <p>每次提问都会检索本班全部材料，问题越具体，越容易找到对应原文。</p>
          </div>
          <span className="evidence-pill"><Icon name="shield" size={15} />仅依据本班资料</span>
        </div>

        <label htmlFor="q" className="composer-label">你的问题</label>
        <textarea
          id="q"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="例如：三羧酸循环发生在细胞的什么部位？"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              ask()
            }
          }}
        />

        <div className="composer-footer">
          <span className="keyboard-hint"><kbd>Ctrl</kbd> / <kbd>⌘</kbd> + <kbd>Enter</kbd> 发送</span>
          <div className="composer-actions">
            {streaming && (
              <button className="ghost stop-button" onClick={stop}>
                <Icon name="stop" size={15} />停止
              </button>
            )}
            <button className="primary ask-button" onClick={ask} disabled={streaming || !question.trim()}>
              {streaming ? <span className="spinner" /> : <Icon name="send" size={17} />}
              {streaming ? '生成中…' : '提问'}
            </button>
          </div>
        </div>

        {phase === 'idle' && !question && (
          <div className="suggestions">
            <span>可以这样问</span>
            <div>{SUGGESTIONS.map((item) => <button key={item} onClick={() => setQuestion(item)}>{item}</button>)}</div>
          </div>
        )}
        {error && <div className="error composer-error" role="alert"><Icon name="alert" size={17} />{error}</div>}
      </section>

      {(streaming || answer || noEvidence) && (
        <section className="answer-card" aria-busy={streaming}>
          <div className="answer-header">
            <div className="answer-title">
              <span className="section-icon answer-icon"><Icon name="quote" size={21} /></span>
              <div><span className="eyebrow">ANSWER</span><h2>回答</h2></div>
            </div>
            <div className="answer-state">{stateLabel()}</div>
          </div>

          {streaming && !answer && (
            <div className="answer-waiting" role="status">
              <span /><span /><span />
              <p>正在检索本班材料并组织回答…</p>
            </div>
          )}
          {answer && <div className="answer">{renderAnswer(answer)}</div>}

          {citations.length > 0 && (
            <div className="citations-panel">
              <div className="citations-heading">
                <h3>引用来源 <span>{citations.length}</span></h3>
                <p>{openCite === null ? '选择一条引用，聚焦核对完整原文切片。' : '再次选择当前引用，可返回全部来源。'}</p>
              </div>
              <div className="list">
                {citations.map((c) => {
                  const focused = openCite === c.index
                  return (
                    <article key={c.chunk_id} className={`cite-item ${focused ? 'focused' : ''}`}>
                      <button
                        type="button"
                        className="cite-source-button"
                        onClick={() => toggleCitation(c.index)}
                        aria-expanded={focused}
                      >
                        <span className="cite-number">{c.index}</span>
                        <strong>《{c.material_title}》</strong>
                        <Icon name="chevron" size={17} />
                      </button>
                      {(openCite === null || focused) && <div className="cite-snippet">{c.snippet}</div>}
                    </article>
                  )
                })}
              </div>
            </div>
          )}
        </section>
      )}
    </div>
  )
}
