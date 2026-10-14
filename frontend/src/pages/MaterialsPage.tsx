import { useEffect, useRef, useState } from 'react'
import { listMaterials, reindex, uploadFile, uploadText, type Material, type User } from '../api'

export default function MaterialsPage({ user }: { user: User }) {
  const [items, setItems] = useState<Material[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  const isTeacher = user.role === 'teacher'

  const refresh = async () => {
    try {
      setItems(await listMaterials())
      setError('')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  const handleText = async () => {
    setBusy(true)
    setError('')
    setNotice('')
    try {
      const m = await uploadText(title, content)
      setNotice(m.index_status === 'ready' ? `已上传并建立索引：${m.title}` : `已上传，但索引失败：${m.index_error}`)
      setTitle('')
      setContent('')
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const handleFile = async (file: File) => {
    setBusy(true)
    setError('')
    setNotice('')
    try {
      const m = await uploadFile(title || file.name, file)
      setNotice(m.index_status === 'ready' ? `已上传并建立索引：${m.title}` : `已上传，但索引失败：${m.index_error}`)
      setTitle('')
      if (fileRef.current) fileRef.current.value = ''
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const handleReindex = async (id: number) => {
    try {
      const m = await reindex(id)
      setNotice(m.index_status === 'ready' ? '重索引成功' : `重索引仍失败：${m.index_error}`)
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const statusText = (s: Material['index_status']) =>
    s === 'ready' ? '可检索' : s === 'failed' ? '索引失败' : '处理中'

  return (
    <>
      <div className="card">
        <h2>本班教学材料（{user.class_name}）</h2>
        <p className="hint">材料按班级隔离：这里只会出现你所在班级的材料。</p>
        <table>
          <thead>
            <tr>
              <th>标题</th>
              <th>索引状态</th>
              <th>上传时间</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {items.map((m) => (
              <tr key={m.id}>
                <td>{m.title}</td>
                <td>
                  <span className={`status ${m.index_status}`}>{statusText(m.index_status)}</span>
                </td>
                <td>{new Date(m.created_at).toLocaleString()}</td>
                <td>
                  {isTeacher && m.index_status === 'failed' && (
                    <button className="ghost" onClick={() => handleReindex(m.id)}>
                      重试索引
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {items.length === 0 && (
              <tr>
                <td colSpan={4} className="hint">
                  暂无材料
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {isTeacher && (
        <div className="card">
          <h2>上传材料</h2>
          <p className="hint">支持纯文本 / Markdown / PDF（PDF 会抽取文本）。学生账号无上传权限。</p>
          <label htmlFor="t">标题</label>
          <input id="t" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="例如：第一节讲义" />
          <label htmlFor="c">正文（纯文本 / Markdown）</label>
          <textarea id="c" value={content} onChange={(e) => setContent(e.target.value)} />
          <button className="primary" onClick={handleText} disabled={busy || !content.trim()}>
            上传文本
          </button>

          <label htmlFor="f">或选择文件（.txt / .md / .pdf）</label>
          <input
            id="f"
            type="file"
            ref={fileRef}
            accept=".txt,.md,.markdown,.pdf"
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (f) void handleFile(f)
            }}
          />
          {notice && <p className="hint">{notice}</p>}
          {error && <div className="error">{error}</div>}
        </div>
      )}
    </>
  )
}
