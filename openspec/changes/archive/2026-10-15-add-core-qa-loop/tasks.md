# Tasks

> 按顺序落地；每条以可判定的 `verify:` 说明"怎么算完成"。完成一条勾一条。
> 服务对象以 Requirement 编号标注（R1–R7 见 `specs/*/spec.md`）。
>
> 验收证据（复现方式见根 `README.md`「本地运行」）：
> - Go 端到端：`cd backend && go run ./cmd/e2e` → **41/41 通过，退出码 0**
> - 前端端到端：`node scripts/e2e-ui.mjs` → **13/13 通过，退出码 0**
> - 一键启动：`cd deploy && cp .env.example .env && docker compose up -d --build` → `/health` 200

## 1. 骨架与一键启动

- [x] 1.1 建立仓库目录结构（`backend/` `frontend/` `deploy/`），写最小 README 跑法
  - verify: 新环境按 README 三步内可启动 → README「本地运行」节已写，后端可本地起并探活
- [x] 1.2 写 `docker-compose.yml`：前端 / 后端 / MySQL / Qdrant / 反向代理（服务 R1–R7 的地基）
  - verify: `docker compose up -d` 后全部容器 `healthy`
  - 实测：`docker compose up -d --build` 三容器 up，`deploy-mysql-1` 状态 `healthy`；`/health` 200 `{"deps":{"mysql":"ok","qdrant":"ok"}}`。
    前端与后端**同源单端口**（Go `embed` 内嵌前端产物），故未引入独立反向代理服务（design D1 垂直切片的自然结果）。
- [x] 1.3 后端进程启动即连上 MySQL 与 Qdrant，失败即退出并打日志
  - verify: 停掉 Qdrant 后启动后端，日志有明确错误且进程退出 → 实测：两依赖均不可用时日志逐项报错、退出码 1
- [x] 1.4 健康检查接口 `GET /health`
  - verify: 返回 200；依赖不可用时返回非 200
  - 实测：容器环境依赖就绪 → 200；把 `QDRANT_URL` 指向死端口 → 503 且逐项列明原因

## 2. 登录与登录态（R1 / R2 / R4）

- [x] 2.1 建用户 / 班级 / 刷新令牌表与种子数据（教师、学生各一，分属两个班）
  - verify: 种子脚本可重复执行且幂等；刷新令牌表只存摘要、无明文令牌列
  - 实测：启动时 `Migrate`+`Seed`，幂等键（用户名 / 班级名）；`refresh_tokens.token_hash CHAR(64)` 仅存 SHA-256 摘要
- [x] 2.2 口令哈希存储（bcrypt/argon2），不存明文
  - verify: 库中口令列为哈希；无明文口令出现
- [x] 2.3 `POST /api/login` 同时签发 access token 与 refresh token（R1.1–R1.4）
  - verify: 正确凭据 200 且含 `access_token` 与 `refresh_token`；错误凭据 401（R1.1 / R1.2）
- [x] 2.4 认证中间件：无 / 坏 access token → 401
  - verify: 篡改一字节后请求受保护接口 → 401（R1.3）；无头 → 401（R1.4）
- [x] 2.5 `POST /api/refresh`：凭 refresh 换取新 access，并轮换 refresh（R1.6 / R1.7）
  - verify: 刷新返回新 `access_token` 与新 `refresh_token`；再用旧 refresh 刷新 → 401（R1.6 / R1.7）
- [x] 2.6 登出：作废该用户的 refresh token（R1.5）
  - verify: 登出后用其 refresh 刷新 → 401；原 access 在自身有效期内仍可用（R1.5）
- [x] 2.7 班级隔离中间件：跨班读一律拒绝（R2.1–R2.3）
  - verify: 用 A 班 token 取 B 班资源 → 403/404 且响应无 B 内容（R2.1）
- [x] 2.8 前端登录页 + 双令牌保存 + 401 静默刷新（R1.1 / R1.6）
  - verify: 登录 → 进主页；刷新页面会话保持；access 过期后自动续期且原请求完成（可临时将有效期调至秒级验证）；并发请求只触发一次刷新
  - 实测：UI T2/T7b/T7c 验证静默续期并重放成功；并发去重由模块级互斥 Promise 保证（未单独压测）

## 3. 材料上传（R3 / R4）

- [x] 3.1 建材料 / 切片表，含 `index_status`
  - verify: 表结构含状态列，默认非 `ready`
- [x] 3.2 `POST /api/materials` 上传（仅教师），落库原文（R3.1 / R3.2）
  - verify: 教师上传非空内容 → 201；空内容 → 400 且不落库（R3.2）
- [x] 3.3 切片 + 入 Qdrant，成功置 `ready`，失败置 `failed`（R3.3）
  - verify: 上传后检索独有词能命中（R3.1）；关掉 Qdrant 后上传 → `failed`（R3.3）
  - 实测：正常上传 `index_status=ready`；把 `QDRANT_URL` 指向死端口 → `failed` 且带明确 `index_error`
- [x] 3.4 重索引接口 `POST /api/materials/{id}/reindex`
  - verify: 对 `failed` 材料重索引后转 `ready`
- [x] 3.5 学生调用上传 → 403（R2.3）
  - verify: 学生 token 上传接口返回 403
- [x] 3.6 前端材料页：列表 + 上传 + 状态显示
  - verify: 上传后列表出现该材料及状态
  - 实测：UI T3

## 4. 检索问答与引用（R5 / R6 / R7 / R4）

- [x] 4.1 检索服务：仅在本班 `ready` 材料内取前 N 片段（R2.2 / R5.1）
  - verify: 只存在于 B 班的内容，A 班学生检索结果为空（R2.2）
- [x] 4.2 组装提示词调用 LLM，落问答记录
  - verify: 命中问答能返回带引用清单的回答（R7.1）
- [x] 4.3 未命中分支：固定文案 + `no_evidence`，不调模型编造（R5.2 / R5.3）
  - verify: 提材料之外的问题 → 固定文案 + `no_evidence`；空问题 → 400（R5.3）
- [x] 4.4 `POST /api/ask` 以 SSE 流式返回（R6）
  - verify: `Content-Type: text/event-stream`；首字节早于整体完成（R6.1）
- [x] 4.5 **透传 `http.Flusher`**（若有包装 ResponseWriter 的中间件）——见 design D6
  - verify: 用 `curl -N` 能观察到分次到达；若一次性到达则为假流式，须修
  - 实测：`statusRecorder` 实现 `Flush()` 与 `Unwrap()`；`curl -N` 可见 `meta`／`delta` 分次到达
- [x] 4.6 客户端断开处理（R6.3）
  - verify: 中途断开连接，服务端停止生成且不留半条记录
  - 实测：Go e2e「R6.3 客户端中途断开」——收到首个 `delta` 即断开，服务端留「客户端中断」痕迹且 `conversations` 不增行
- [x] 4.7 前端问答页：输入 → 流式渲染 → `[n]` 可点开引用片段（R7.1）
  - verify: 点击 `[n]` 弹出对应材料片段文本
  - 实测：UI T4/T4b/T4c/T5

## 5. 地基收口与验收证据（R4 + 课纲）

- [x] 5.1 审计留痕：登录 / 上传 / 问答各写一条（R4.1 / R4.2）
  - verify: 三类动作后审计表各增一条；登录失败亦留痕且不含明文口令（R4.2）
- [x] 5.2 一键启动 + 健康检查纳入验收脚本
  - verify: 全新环境一条命令拉起并探活通过
  - 实测：`docker compose up -d --build` 后 `/health` 200（Go e2e 首项断言）
- [x] 5.3 端到端验收脚本：登录 → 上传 → 问答 → 引用，含跨班负向用例
  - verify: 脚本退出码为判定；全绿即通过（失败路径覆盖 R2.1 / R2.2 / R2.3）
  - 实测：Go e2e 41 项、UI e2e 13 项，均 0 失败、退出码 0

## 6. 归档

- [x] 6.1 `openspec validate --all` 通过
  - verify: 无错误输出 → `validate --all --strict` 通过
- [x] 6.2 归档本 change，`specs/*` 并入主规约
  - verify: `openspec/specs/` 出现两个 capability；change 移入 `archive/`
