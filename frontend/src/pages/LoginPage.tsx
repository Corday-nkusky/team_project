import { useState } from 'react'
import { login, type User } from '../api'
import Icon from '../components/Icon'

const DEMO: Array<{ label: string; username: string; password: string; role: string }> = [
  { label: '1 班教师', username: 'teacher1', password: 'teacher123', role: '上传与管理材料' },
  { label: '1 班学生', username: 'student1', password: 'student123', role: '提问与查看引用' },
  { label: '2 班教师', username: 'teacher2', password: 'teacher123', role: '上传与管理材料' },
  { label: '2 班学生', username: 'student2', password: 'student123', role: '提问与查看引用' },
]

export default function LoginPage({ onSignedIn }: { onSignedIn: (u: User) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const u = await login(username, password)
      onSignedIn(u)
    } catch (err) {
      // 后端对「账号不存在」与「口令错误」返回完全一致的提示（R1.2）。
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const fill = (u: string, p: string) => {
    setUsername(u)
    setPassword(p)
    setError('')
  }

  return (
    <div className="login-shell">
      <section className="login-story" aria-label="产品介绍">
        <div className="brand login-brand">
          <span className="brand-mark"><Icon name="logo" size={29} /></span>
          <span className="brand-copy">
            <strong>CampusClaw</strong>
            <span>班级知识工作台</span>
          </span>
        </div>

        <div className="story-copy">
          <span className="eyebrow">LEARN WITH EVIDENCE</span>
          <h1>让每一次提问，<br /><em>都有出处。</em></h1>
          <p>把教师的讲义变成班级专属知识库。学生得到的不是泛泛回答，而是能回到原文的学习线索。</p>
        </div>

        <div className="story-points">
          <div><Icon name="class" /><span><strong>班级边界</strong><small>只检索自己班级的教学材料</small></span></div>
          <div><Icon name="quote" /><span><strong>原文引用</strong><small>回答中的每处依据都可展开核对</small></span></div>
          <div><Icon name="shield" /><span><strong>诚实作答</strong><small>资料没有依据时，明确说不知道</small></span></div>
        </div>

        <p className="story-footer">TEACH · ASK · UNDERSTAND</p>
      </section>

      <main className="login-panel">
        <div className="login-form-wrap">
          <div className="login-heading">
            <span className="eyebrow">WELCOME BACK</span>
            <h2>登录 CampusClaw</h2>
            <p>进入你的班级知识工作台</p>
          </div>

          <form onSubmit={submit} className="login-form">
            <div className="field">
              <label htmlFor="u">账号</label>
              <div className="input-with-icon">
                <Icon name="user" size={18} />
                <input
                  id="u"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  placeholder="输入账号"
                  required
                  autoFocus
                />
              </div>
            </div>
            <div className="field">
              <label htmlFor="p">口令</label>
              <div className="input-with-icon">
                <Icon name="shield" size={18} />
                <input
                  id="p"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                  placeholder="输入口令"
                  required
                />
              </div>
            </div>
            {error && <div className="error" role="alert"><Icon name="alert" size={17} />{error}</div>}
            <button className="primary login-submit" type="submit" disabled={busy || !username || !password}>
              {busy ? <span className="spinner" /> : <Icon name="arrow" size={18} />}
              {busy ? '登录中…' : '登录'}
            </button>
          </form>

          <div className="demo-section">
            <div className="demo-heading"><span>演示账号</span><small>点击填入，不自动登录</small></div>
            <div className="demo-grid">
              {DEMO.map((d) => (
                <button type="button" key={d.username} className="demo-account" onClick={() => fill(d.username, d.password)}>
                  <span className={`demo-icon ${d.username.startsWith('teacher') ? 'teacher' : ''}`}>
                    <Icon name={d.username.startsWith('teacher') ? 'book' : 'class'} size={18} />
                  </span>
                  <span><strong>{d.label}</strong><small>{d.role}</small></span>
                  <Icon name="arrow" size={16} />
                </button>
              ))}
            </div>
          </div>

          <p className="login-footnote"><Icon name="shield" size={15} />账号仅用于课堂演示，请勿填入真实口令。</p>
        </div>
      </main>
    </div>
  )
}
