import { useCallback, useEffect, useState } from 'react'
import { getObservability, type Observability } from '../api'
import Icon from '../components/Icon'

export default function ObservabilityPage() {
  const [data, setData] = useState<Observability | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const reload = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setData(await getObservability())
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void reload() }, [reload])

  if (loading && !data) {
    return <div className="obs-state" role="status"><Icon name="refresh" size={24} /><p>正在汇总本班真实调用记录…</p></div>
  }
  if (error && !data) {
    return (
      <div className="obs-state error-state" role="alert">
        <Icon name="alert" size={24} /><p>{error}</p>
        <button onClick={() => void reload()}>重新加载</button>
      </div>
    )
  }

  const skills = data?.skills ?? []
  const requests = data?.requests ?? []
  const maxCalls = Math.max(1, ...skills.map((item) => item.calls))
  const maxP95 = Math.max(1, ...requests.map((item) => item.p95_ms))
  const isEmpty = skills.length === 0 && requests.length === 0 && data?.queue_depth === 0

  return (
    <div className="obs-page">
      <div className="obs-toolbar">
        <span>最近请求窗口与只增审计记录 · {data ? new Date(data.generated_at).toLocaleString() : ''}</span>
        <button disabled={loading} onClick={() => void reload()}><Icon name="refresh" size={16} />{loading ? '刷新中…' : '刷新'}</button>
      </div>
      {error && <div className="skills-error" role="alert">{error}</div>}
      {isEmpty && (
        <div className="empty-state">
          <Icon name="clock" size={28} />
          <h3>还没有运行数据</h3>
          <p>先完成一次本班提问或材料上传，再回到这里刷新；没有数据时不会生成示例曲线。</p>
        </div>
      )}

      <div className="obs-grid">
        <section className="obs-card">
          <header><span className="section-icon"><Icon name="spark" size={19} /></span><div><h2>技能调用量</h2><p>按生效技能分线，含基础问答</p></div></header>
          {skills.length === 0 ? <p className="empty">尚无问答留痕。</p> : skills.map((item) => (
            <div className="obs-row" key={item.skill_name}>
              <div className="obs-row-label"><strong>{item.skill_name}</strong><span>{item.calls} 次 · 用量 {item.usage_count}</span></div>
              <div className="obs-track" aria-label={`${item.skill_name} 调用 ${item.calls} 次`}><span style={{ width: `${item.calls / maxCalls * 100}%` }} /></div>
            </div>
          ))}
        </section>

        <section className="obs-card">
          <header><span className="section-icon"><Icon name="alert" size={19} /></span><div><h2>每技能错误率</h2><p>失败与中断数 ÷ 调用数</p></div></header>
          {skills.length === 0 ? <p className="empty">尚无错误率样本。</p> : skills.map((item) => (
            <div className="obs-error-item" key={item.skill_name}>
              <span>{item.skill_name}</span><strong>{(item.error_rate * 100).toFixed(1)}%</strong>
              <small>{item.errors} / {item.calls} 次失败</small>
            </div>
          ))}
        </section>

        <section className="obs-card obs-requests">
          <header><span className="section-icon"><Icon name="clock" size={19} /></span><div><h2>接口 P95 与队列深度</h2><p>每路由最近 256 个进程内样本</p></div></header>
          <div className="queue-depth"><span>本班索引队列</span><strong>{data?.queue_depth ?? 0}</strong><small>pending + running</small></div>
          {requests.length === 0 ? <p className="empty">本进程尚无请求样本。</p> : requests.map((item) => (
            <div className="obs-request-row" key={item.route}>
              <div><code>{item.route}</code><span>{item.calls} 次 · {item.errors} 个服务错误</span></div>
              <div className="obs-track"><span style={{ width: `${item.p95_ms / maxP95 * 100}%` }} /></div>
              <strong>{item.p95_ms.toFixed(2)} ms</strong>
            </div>
          ))}
        </section>
      </div>
    </div>
  )
}
