import { useState } from 'react'
import { login, type User } from '../api'

const DEMO: Array<{ label: string; username: string; password: string }> = [
  { label: '1 班教师', username: 'teacher1', password: 'teacher123' },
  { label: '1 班学生', username: 'student1', password: 'student123' },
  { label: '2 班教师', username: 'teacher2', password: 'teacher123' },
  { label: '2 班学生', username: 'student2', password: 'student123' },
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
  }

  return (
    <div className="app" style={{ maxWidth: 420, paddingTop: 80 }}>
      <div className="card">
        <h2>登录 CampusClaw</h2>
        <form onSubmit={submit}>
          <label htmlFor="u">账号</label>
          <input id="u" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
          <label htmlFor="p">口令</label>
          <input
            id="p"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
          {error && <div className="error">{error}</div>}
          <button className="primary" type="submit" disabled={busy || !username || !password}>
            {busy ? '登录中…' : '登录'}
          </button>
        </form>
      </div>

      <div className="card">
        <h2>演示账号</h2>
        <p className="hint">点击填入（由种子数据创建，两个班互相看不到对方材料）。</p>
        <div className="tabs" style={{ flexWrap: 'wrap' }}>
          {DEMO.map((d) => (
            <button key={d.username} className="ghost" onClick={() => fill(d.username, d.password)}>
              {d.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
