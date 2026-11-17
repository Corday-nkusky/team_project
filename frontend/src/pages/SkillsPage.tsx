import { useCallback, useEffect, useState } from 'react'
import {
  connectMCPServer, deleteMCPServer, deleteSkill, disconnectMCPServer,
  listMCPCalls, listMCPServers, listMCPTools, listSkills, saveMCPServer, saveSkill,
  type MCPCall, type MCPServer, type MCPTool, type Skill,
} from '../api'
import Icon from '../components/Icon'

type SubTab = 'skills' | 'servers' | 'calls'

const emptyDraft = {
  name: '解题引导', instruction: '先问学生卡在哪一步，再给一个提示，不直接给答案。',
  enabled: true, tool_refs: '',
}

export default function SkillsPage() {
  const [sub, setSub] = useState<SubTab>('skills')
  const [skills, setSkills] = useState<Skill[]>([])
  const [servers, setServers] = useState<MCPServer[]>([])
  const [calls, setCalls] = useState<MCPCall[]>([])
  const [tools, setTools] = useState<Record<number, MCPTool[]>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const [draft, setDraft] = useState(emptyDraft)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [serverDraft, setServerDraft] = useState({ name: 'campus-tools', base_url: 'http://mcp-server:8090', auth_token: '' })

  const reload = useCallback(async () => {
    setError('')
    try {
      const [s, sv, c] = await Promise.all([listSkills(), listMCPServers(), listMCPCalls(20)])
      setSkills(s)
      setServers(sv)
      setCalls(c)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  useEffect(() => { void reload() }, [reload])

  const wrap = async (fn: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try { await fn() } catch (e) { setError((e as Error).message) }
    finally { setBusy(false) }
  }

  const submitSkill = () => wrap(async () => {
    await saveSkill({
      id: editingId ?? undefined,
      name: draft.name.trim(),
      kind: 'solve_hint',
      enabled: draft.enabled,
      instruction: draft.instruction.trim(),
      tool_refs: draft.tool_refs.split(/[,，\n]/).map((s) => s.trim()).filter(Boolean),
    })
    setDraft(emptyDraft)
    setEditingId(null)
    await reload()
  })

  const editSkill = (sk: Skill) => {
    setEditingId(sk.id)
    setDraft({
      name: sk.name, instruction: sk.instruction, enabled: sk.enabled,
      tool_refs: sk.tool_refs.join(', '),
    })
  }

  const submitServer = () => wrap(async () => {
    await saveMCPServer({
      name: serverDraft.name.trim(),
      base_url: serverDraft.base_url.trim(),
      ...(serverDraft.auth_token.trim() ? { auth_token: serverDraft.auth_token.trim() } : {}),
    })
    setServerDraft((d) => ({ ...d, auth_token: '' }))
    await reload()
  })

  const connect = (id: number) => wrap(async () => {
    const res = await connectMCPServer(id)
    setTools((prev) => ({ ...prev, [id]: res.tools }))
    await reload()
  })

  const showTools = async (id: number) => wrap(async () => {
    const list = await listMCPTools(id)
    setTools((prev) => ({ ...prev, [id]: list }))
  })

  return (
    <div className="skills-page">
      <div className="skills-tabs" role="tablist">
        {([['skills', '解题引导技能'], ['servers', 'MCP 工具服务'], ['calls', '调用记录']] as const).map(([id, label]) => (
          <button key={id} role="tab" aria-selected={sub === id}
            className={sub === id ? 'active' : ''} onClick={() => setSub(id)}>{label}</button>
        ))}
      </div>

      {error && <div className="skills-error" role="alert">{error}</div>}

      {sub === 'skills' && (
        <div className="skills-grid">
          <section className="skills-form">
            <h3>{editingId ? '编辑技能' : '新建解题引导技能'}</h3>
            <label>名称<input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} /></label>
            <label>引导提示词<textarea rows={5} value={draft.instruction}
              onChange={(e) => setDraft({ ...draft, instruction: e.target.value })} /></label>
            <label>工具引用（逗号分隔，如 formula_hint@campus-tools）
              <input value={draft.tool_refs} onChange={(e) => setDraft({ ...draft, tool_refs: e.target.value })} /></label>
            <label className="inline"><input type="checkbox" checked={draft.enabled}
              onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} />启用</label>
            <div className="skills-actions">
              <button disabled={busy} onClick={submitSkill}>{editingId ? '保存修改' : '保存技能'}</button>
              {editingId && <button disabled={busy} onClick={() => { setEditingId(null); setDraft(emptyDraft) }}>取消</button>}
            </div>
            <p className="hint">关闭技能后，同一道题的回答会回到未接入技能的形态，可用于课堂演示开关差异。</p>
          </section>
          <section className="skills-list">
            <h3>本班技能（{skills.length}）</h3>
            {skills.length === 0 && <p className="empty">还没有配置技能。</p>}
            {skills.map((sk) => (
              <article key={sk.id} className="skill-item">
                <header>
                  <strong>{sk.name}</strong>
                  <span className={sk.enabled ? 'tag on' : 'tag off'}>{sk.enabled ? '已启用' : '已关闭'}</span>
                  <span className="version">v{sk.version}</span>
                </header>
                <p>{sk.instruction}</p>
                {sk.tool_refs.length > 0 && <p className="refs">工具：{sk.tool_refs.join('、')}</p>}
                <footer>
                  <button disabled={busy} onClick={() => editSkill(sk)}>编辑</button>
                  <button disabled={busy}
                    onClick={() => wrap(async () => { await deleteSkill(sk.id); await reload() })}>删除</button>
                </footer>
              </article>
            ))}
          </section>
        </div>
      )}

      {sub === 'servers' && (
        <div className="skills-grid">
          <section className="skills-form">
            <h3>登记工具服务</h3>
            <label>服务名称<input value={serverDraft.name}
              onChange={(e) => setServerDraft({ ...serverDraft, name: e.target.value })} /></label>
            <label>Base URL<input value={serverDraft.base_url}
              onChange={(e) => setServerDraft({ ...serverDraft, base_url: e.target.value })} /></label>
            <label>访问令牌（只写不回，留空表示不改动）<input type="password" value={serverDraft.auth_token}
              onChange={(e) => setServerDraft({ ...serverDraft, auth_token: e.target.value })} /></label>
            <div className="skills-actions"><button disabled={busy} onClick={submitServer}>登记 / 更新</button></div>
            <p className="hint">令牌保存在服务端，列表只显示「是否已配置」，不会回传明文。连接失败不影响检索与问答。</p>
          </section>
          <section className="skills-list">
            <h3>已登记服务（{servers.length}）</h3>
            {servers.length === 0 && <p className="empty">还没有登记工具服务。</p>}
            {servers.map((sv) => (
              <article key={sv.id} className="skill-item">
                <header>
                  <strong>{sv.name}</strong>
                  <span className={sv.connected ? 'tag on' : 'tag off'}>{sv.connected ? '已连接' : '未连接'}</span>
                  <span className="version">{sv.token_configured ? '令牌已配置' : '无令牌'}</span>
                </header>
                <p className="refs">{sv.base_url}</p>
                {sv.last_error && <p className="error-line">上次错误：{sv.last_error}</p>}
                {tools[sv.id] && (
                  <ul className="tool-list">{tools[sv.id].map((t) => (
                    <li key={t.name}><strong>{t.name}</strong> — {t.description}</li>
                  ))}</ul>
                )}
                <footer>
                  <button disabled={busy} onClick={() => connect(sv.id)}>连接 / 发现</button>
                  <button disabled={busy} onClick={() => showTools(sv.id)}>查看工具</button>
                  {sv.connected && <button disabled={busy}
                    onClick={() => wrap(async () => { await disconnectMCPServer(sv.id); await reload() })}>断开</button>}
                  <button disabled={busy}
                    onClick={() => wrap(async () => { await deleteMCPServer(sv.id); await reload() })}>删除</button>
                </footer>
              </article>
            ))}
          </section>
        </div>
      )}

      {sub === 'calls' && (
        <section className="skills-list">
          <h3>调用记录（本班，{calls.length} 条）</h3>
          {calls.length === 0 && <p className="empty">还没有工具调用记录。</p>}
          {calls.map((c) => (
            <article key={c.id} className="call-item">
              <header>
                <strong>{c.tool_ref}</strong>
                <span className={`tag ${c.result === 'ok' ? 'on' : 'off'}`}>
                  {c.result === 'ok' ? '成功' : c.result === 'denied' ? '被拒' : '出错'}
                </span>
                <span className="version">第 {c.round} 轮 · {c.created_at}</span>
              </header>
              <p>{c.actor_name}{c.conversation_id ? ` · 会话 ${c.conversation_id}` : ''}{c.detail ? ` · ${c.detail}` : ''}</p>
            </article>
          ))}
          <p className="hint"><Icon name="shield" size={15} /> 调用记录只增不改，仅本班教师可见。</p>
        </section>
      )}
    </div>
  )
}
