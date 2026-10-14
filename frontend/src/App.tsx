import { useEffect, useState } from 'react'
import { clearSession, getCurrentUser, logout, setUnauthorizedHandler, type User } from './api'
import LoginPage from './pages/LoginPage'
import MaterialsPage from './pages/MaterialsPage'
import AskPage from './pages/AskPage'

type Tab = 'ask' | 'materials'

export default function App() {
  const [user, setUser] = useState<User | null>(() => getCurrentUser())
  const [tab, setTab] = useState<Tab>('ask')

  // 续期彻底失败时回到登录页。
  useEffect(() => {
    setUnauthorizedHandler(() => setUser(null))
  }, [])

  if (!user) {
    return <LoginPage onSignedIn={setUser} />
  }

  const handleLogout = async () => {
    await logout()
    clearSession()
    setUser(null)
  }

  return (
    <div className="app">
      <div className="topbar">
        <h1>CampusClaw 智教闭环</h1>
        <span className="badge">{user.role === 'teacher' ? '教师' : '学生'}</span>
        <div className="spacer" />
        <span className="who">
          {user.display_name}（{user.class_name}）
        </span>
        <button className="ghost" onClick={handleLogout}>
          退出登录
        </button>
      </div>

      <div className="tabs">
        <button className={tab === 'ask' ? 'active' : ''} onClick={() => setTab('ask')}>
          提问问答
        </button>
        <button
          className={tab === 'materials' ? 'active' : ''}
          onClick={() => setTab('materials')}
        >
          教学材料
        </button>
      </div>

      {tab === 'ask' ? <AskPage /> : <MaterialsPage user={user} />}
    </div>
  )
}
