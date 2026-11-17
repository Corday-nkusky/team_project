import { useEffect, useState } from 'react'
import { clearSession, getCurrentUser, logout, setUnauthorizedHandler, type User } from './api'
import Icon, { type IconName } from './components/Icon'
import LoginPage from './pages/LoginPage'
import MaterialsPage from './pages/MaterialsPage'
import AskPage from './pages/AskPage'
import SkillsPage from './pages/SkillsPage'

type Tab = 'ask' | 'materials' | 'skills'

const NAV: Array<{ id: Tab; label: string; icon: IconName }> = [
  { id: 'ask', label: '提问问答', icon: 'ask' },
  { id: 'materials', label: '教学材料', icon: 'materials' },
  { id: 'skills', label: '技能与工具', icon: 'shield' },
]

export default function App() {
  const [user, setUser] = useState<User | null>(() => getCurrentUser())
  const [tab, setTab] = useState<Tab>('ask')
  const [loggingOut, setLoggingOut] = useState(false)

  // 续期彻底失败时回到登录页。
  useEffect(() => {
    setUnauthorizedHandler(() => setUser(null))
  }, [])

  if (!user) {
    return <LoginPage onSignedIn={setUser} />
  }

  const handleLogout = async () => {
    if (loggingOut) return
    setLoggingOut(true)
    try {
      await logout()
    } catch {
      // 网络失败也允许本地退出；远端 refresh 已失效或将在到期后失效。
    } finally {
      clearSession()
      setUser(null)
      setLoggingOut(false)
    }
  }

  // 技能与工具是教师配置项，学生不显示入口。
  const nav = NAV.filter((item) => item.id !== 'skills' || user.role === 'teacher')
  const activeTab: Tab = tab === 'skills' && user.role !== 'teacher' ? 'ask' : tab
  const heading = {
    ask: ['GROUNDED Q&A', '从班级资料里，找到答案', '每一个回答都有材料依据，每一处引用都能回看。'],
    materials: ['CLASS LIBRARY', '班级材料，有序沉淀', '上传、索引与检索，都严格限定在当前班级。'],
    skills: ['AI CAPABILITIES', '技能与 MCP 工具', '配置解题引导与工具服务，所有操作按班级隔离。'],
  }[activeTab]
  const initial = [...user.display_name][0] ?? '用'

  return (
    <div className="workspace">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark"><Icon name="logo" size={27} /></span>
          <span className="brand-copy">
            <strong>CampusClaw</strong>
            <span>班级知识工作台</span>
          </span>
        </div>

        <div className="nav-caption">WORKSPACE</div>
        <nav className="main-nav" aria-label="主导航">
          {nav.map((item) => (
            <button
              key={item.id}
              className={`nav-item ${activeTab === item.id ? 'active' : ''}`}
              onClick={() => setTab(item.id)}
              aria-current={activeTab === item.id ? 'page' : undefined}
            >
              <Icon name={item.icon} size={20} />
              <span>{item.label}</span>
              <span className="nav-indicator" />
            </button>
          ))}
        </nav>

        <div className="sidebar-note">
          <Icon name="shield" size={21} />
          <div>
            <strong>答案有出处</strong>
            <p>只检索本班材料，引用可回到原文。</p>
          </div>
        </div>

        <div className="sidebar-user">
          <div className="user-summary">
            <span className="avatar">{initial}</span>
            <span className="user-meta">
              <strong>{user.display_name}</strong>
              <span>{user.class_name}</span>
            </span>
          </div>
          <button className="logout-button" onClick={handleLogout} disabled={loggingOut}>
            <Icon name="logout" size={18} />
            <span>{loggingOut ? '正在退出…' : '退出登录'}</span>
          </button>
        </div>
      </aside>

      <main className="workspace-main">
        <header className="workspace-header">
          <div className="page-heading">
            <span className="eyebrow">{heading[0]}</span>
            <h1>{heading[1]}</h1>
            <p>{heading[2]}</p>
          </div>
          <div className="header-context">
            <span className="class-pill"><Icon name="class" size={17} />{user.class_name}</span>
            <span className="badge">{user.role === 'teacher' ? '教师' : '学生'}</span>
          </div>
        </header>

        <div className="page-content">
          {activeTab === 'ask' && <AskPage />}
          {activeTab === 'materials' && <MaterialsPage user={user} />}
          {activeTab === 'skills' && <SkillsPage />}
        </div>
      </main>
    </div>
  )
}
