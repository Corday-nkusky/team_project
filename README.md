# CampusClaw 智教闭环

> 基于本班材料、原文引用与教学工具编排的班级智能问答系统
> —— 北京大学「互联网软件开发技术与实践」课程小组项目

## 项目定位

面向**班级教学场景**，本仓**从零构建**一套完整系统，不基于既有系统做增量：教师上传材料，学生在**本班范围**内提问并获得带原文引用的流式回答；资料没有依据时如实回答「资料中未找到相关内容」。系统同时提供历史问答、解题引导技能、MCP 工具编排和教师运行观测。

> 课程中出现的 CampusClaw 演示系统只作问题域参考；本仓的认证、权限、检索与工具编排均自行实现。观测数据来自真实请求与只增审计，不把调用次数等同于学习效果。

## 核心功能模块

| 模块 | 做什么 |
|------|--------|
| **班级材料与异步索引** | 教师上传文本、Markdown 或 PDF 文本；系统按班保存原文和结构切片，通过独立 worker 建立词面与向量索引，任务状态可轮询、失败可重索引。 |
| **有引用的流式问答** | 学生在本班 ready 材料中检索并接收 SSE 回答；引用可回原文，未命中固定拒答。本人可查看本班历史问答。 |
| **技能与 MCP 工具** | 教师配置可版本化、可开关的解题引导技能；登记并连接 MCP 服务，由模型决策、宿主校验和执行工具，调用记录只增不改。 |
| **教师审计与运行观测** | 教师查询本班操作留痕，查看技能调用量、每技能错误率、路由 P95 与索引队列深度；学生访问教师接口被拒绝并留痕。 |

### 使用流程

```
教师上传材料 → worker 异步索引 → 学生提问并获得引用回答
   → 教师按需配置技能与工具 → 审计和运行观测回查真实行为
```

## 技术方案

**系统架构**：React + TypeScript 前端 / Go REST 与 SSE API / MySQL 8.4 业务数据 / Qdrant 向量检索 / OpenAI 兼容模型网关 / Docker Compose。Web 与索引 worker 是两个可独立启停的进程，共用同一镜像与外置配置。前端产物由 Go `embed` 内嵌，与接口**同源单端口**，认证、RBAC 及班级隔离由本仓实现。

| 模块 | 技术栈 | 实现方案 | 验收目标 |
|------|--------|----------|----------|
| 材料与知识库 | Go、MySQL、Qdrant、异步 worker | 结构切片、BM25、向量、RRF 与精排；写库和任务入队使用事务，重复提交受唯一约束保护。 | 上传可追踪、跨班不可见、失败可恢复、引用可回溯。 |
| 智能体问答 | Go、SSE、OpenAI 兼容网关 | 班级权限、混合检索、技能与工具编排、历史问答；无模型密钥时抽取式降级。 | 流式事件完整、无依据固定拒答、回答有出处。 |
| 技能与 MCP 工具 | Go、JSON-RPC 2.0、MySQL | 教师按班配置技能；自建 `mcp-server` 暴露工具，宿主登记、发现、校验、执行并记录。 | 技能开关可演示，工具结果可审计，令牌不回传。 |
| 审计与观测 | Go、MySQL、React | additive migration 扩展审计字段；真实聚合技能指标，进程内按路由保存最近 256 个耗时样本。 | 七列留痕可回查，三类图无示例数据，学生 403 留痕。 |
| 测试与部署 | Go 测试、Node 脚本、Docker Compose | 质量与安全门禁、接口/浏览器 e2e、真实告警、同参数压测、一键启动与版本探活。 | 命令以退出码判定，结果可复现，镜像版本可反查。 |

## 课程要求覆盖

> 课纲要求本身混合了**三类不同性质**的东西——用户可见的功能、工程与交付要求、AI 能力。
> 下表按类别分组登记"落在哪一轮"。**本表属项目级**（不写进任何单个 change 的 proposal）；「后续轮次」为**草排，待审**。

**A. 用户可见的功能**

| 要求 | 第 1 轮（`add-core-qa-loop`） | 后续轮次（草排） |
|---|---|---|
| 登录 | ✅ 账号口令登录、区分教师 / 学生 | |
| 数据持久化 | ✅ 用户 / 班级 / 材料 / 切片 / 会话入库 | |
| 接口 | ✅ 登录、材料上传、检索问答（含引用） | |
| 权限 | ✅ 班级隔离（跨班数据不可见） | |
| 业务状态 | ✅ 索引任务状态、材料索引状态、技能开关与版本 | |

**B. 工程与交付要求**（不是功能，是"怎么做到"与交付约束）

| 要求 | 落点 |
|---|---|
| 测试 | 第 1 轮：端到端验收脚本（含权限负向用例）；此后持续补用例 |
| 审计 | 登录 / 上传 / 问答 / 越权拒绝留痕；教师按班查询，记录只增不改 |
| 部署 | 第 1 轮：Compose 一键启动 + 探活 |
| 异步任务 | **不单列为功能**，而是「上传材料」的实现方式：上传返回 202 + 任务标识，MySQL 任务表作持久化队列，独立 worker 异步建索引（第 8 课要求，已落地） |
| 公网可访问 | **待定**：由团队与教师确认部署方式 |
| 规约驱动 | 第 1 轮起：`openspec/` 四件套，每轮持续 |
| Swagger / 接口测试 | ✅ `/swagger.html` 可试调、`/openapi.json` 全量描述；`apitest/run_all.py` 一键跑并生成 HTML 报告 |

**C. 五项 AI 能力**

| 能力 | 状态 | 落点 |
|---|---|---|
| LLM | ✅ | 据本班材料生成回答（无密钥时抽取式降级） |
| 知识库与 RAG | ✅ | 结构切片 + BM25 倒排 + 向量 + RRF + 精排，引用可回原文 |
| 智能体 | ✅ | 流式问答（SSE）、未命中如实作答、引用可追溯 |
| 技能 | ✅ | 教师按班维护解题引导技能，开关可观测地改变回答；`skills` spec |
| MCP | ✅ | 自建 `mcp-server/`（JSON-RPC 2.0 三方法）+ 宿主登记/连接/发现/调用/记录；`mcp-tools` spec |

### 技能与 MCP 的使用与验证

- **技能**：教师登录后进「技能与工具 → 解题引导技能」，新建或编辑技能；关闭技能后，同一道题的回答回到未接入技能的形态，可当场对比。技能的 tool_refs 填 `工具名@服务器名`，保存前会校验格式、工具是否在发现清单内、服务器是否已连接。
- **MCP**：在「MCP 工具服务」登记服务（Base URL 指向 `http://mcp-server:8090`，令牌只写不回），点「连接 / 发现」完成握手 + 工具发现两步；连接后可在技能里引用其工具。「调用记录」页显示最近 20 条只增不改的记录，含工具、结果、触发人、会话与轮次。
- **接口测试**：`python apitest/run_all.py`（需四服务已启动），一键跑登录/技能/MCP/端到端用例并生成 `apitest/report.html`（不入库）。
- **Swagger**：浏览器打开 `/swagger.html`，可在线试调全部接口；离线时 `/openapi.json` 给出完整描述。
- **审计与运行观测**：教师进入「运行观测」查看真实技能调用量、错误率、路由 P95 和本班索引队列；`GET /api/audit` 回查七列留痕。P95 只保留当前进程最近 256 个样本，重启后重新采集，不展示虚构历史曲线。
- **材料列表缓存**：默认 `MATERIAL_CACHE_TTL_MS=2000`，按班隔离、写后失效；设为 0 可关闭并复跑同参数 A/B 压测。
- **15 题评测**：`node scripts/evaluate-qa.mjs` 在真实服务上创建独立 A/B 班材料，执行 5 道有答案、5 道无答案、5 道跨班题并保存 `reports/eval-15.json`。
- **本地告警**：先用 `node scripts/alert-check.mjs --receiver` 启动仅绑定 `127.0.0.1` 的接收器，再按 `reports/alert-triggered.log` 的环境变量运行监控模式；只有接收器明确返回 `delivered=true` 才记为送达。

## 团队分工

本项目为三人小组协作完成，按「规约制定 / 系统开发 / 测试验收」三类职责分工，具体人员安排不在本仓公开。

## 本地运行

前置：一键起全套需 Docker Desktop（含 Compose v2）与 Node.js ≥ 20；只跑后端需 Go 1.27+。

**刚拿到仓库的人怎么跑（三步）**

1. 装齐前置：Docker Desktop（Windows 需 WSL2 后端，首次要能拉镜像）+ Node.js ≥ 20。
2. 启动：Windows 双击根目录 `启动项目.cmd`；macOS / Linux 在终端跑 `node scripts/up.mjs`。
3. 灌数据：仓库不带 MySQL 数据卷，空库只有演示账号、没有课程材料；按下文「演示语料」跑一次 `seed.mjs`。

`启动项目.cmd` / `停止项目.cmd` 只是 Windows 的便利外壳，内部就是 `node scripts/launch.mjs` 与
`node scripts/stop.mjs`，**不自带运行时**：没有 Node.js 时第一屏就报错退出（提示里给了 `winget` 一行命令），
没有 Docker Desktop 会卡在「等待 Docker 引擎」最多 180 秒。macOS / Linux 上双击无效（`.cmd` 是 Windows
批处理），但这两条 `node scripts/...` 是跨平台可用的，`launch.mjs` 会按平台调 `open` / `xdg-open` 打开浏览器。
停止脚本在缺少 `deploy/.env` 时会自动退回 `.env.example` 的占位值，不会因为干净 clone 没有配置而报错。

**一条命令启动整套（推荐）**

```bash
node scripts/up.mjs
```

脚本会生成 `deploy/.env`（首次自动填入随机密钥，含 `MCP_AUTH_TOKEN`）、构建镜像、启动容器，并轮询
`http://127.0.0.1:8080/health`；就绪后打印 `version`、`time` 与依赖状态。
Compose 起五个服务：`mysql` / `qdrant` / `backend`（Web）/ `worker`（索引消费者）/ `mcp-server`（自建工具服务）。
`backend` / `worker` / `mcp-server` 共用同一镜像，只是入口与环境变量不同。
浏览器打开 `http://127.0.0.1:8080` 即为应用（前端由 Go 内嵌，同源单端口）；`/swagger.html` 为接口文档。

手动方式（等价）：

```bash
cd deploy
cp .env.example .env        # 必须把 JWT_SECRET、MCP_AUTH_TOKEN 换成随机长串：openssl rand -hex 32
docker compose up -d --build
curl -s --noproxy '*' http://127.0.0.1:8080/health   # 期望 200
```

**配置纪律**：`HTTP_ADDR`、`MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_USER`、`MYSQL_PASSWORD`、
`MYSQL_DATABASE`、`QDRANT_URL`、`JWT_SECRET` 八项必填，代码内不保留可用默认值。
缺项、空值、非法端口或示例密钥都会让进程**在监听端口前退出**，JSON 日志只报键名、不回显值（spec R8）。
调优项（TTL、切片、阈值、模型名）可选，见 `deploy/.env.example`。

首次构建会拉取 `node:22-alpine` / `golang:1.27` / `mysql:8.4` / `qdrant` 基础镜像，耗时较长；
之后 `docker compose up -d` 秒级启动。

**演示账号**（种子数据，分属两个班，用于验证班级隔离）

| 账号 | 口令 | 角色 | 班级 |
|------|------|------|------|
| `teacher1` | `teacher123` | 教师 | 软件工程1班 |
| `student1` | `student123` | 学生 | 软件工程1班 |
| `teacher2` | `teacher123` | 教师 | 软件工程2班 |
| `student2` | `student123` | 学生 | 软件工程2班 |

**零密钥也能跑通**：`LLM_API_KEY` 留空时，问答退化为「抽取式」回答（直接引用本班材料片段），
检索只走关键词路。启用模型网关时，四项配置必须齐全：

- `LLM_API_BASE`：OpenAI 兼容网关根地址（含 `/v1` 等前缀）；
- `LLM_API_KEY`：网关密钥，只放本机 `deploy/.env`；
- `LLM_CHAT_MODEL`：`POST {base}/chat/completions`，必须支持 `stream=true` 的 SSE；
- `LLM_EMBED_MODEL`：`POST {base}/embeddings`，返回向量维度需固定。

模型名没有默认值，缺失时服务在监听前退出。网关必须同时支持对话与嵌入；只支持对话的网关
（例如 DeepSeek 官方接口）不能直接作为这里的单一网关。

**智谱 v4 的认证**：`https://open.bigmodel.cn/api/paas/v4` 不接受把 Key 直接当 Bearer 令牌，
Key 必须是 `id.secret` 形式；客户端会用 secret 签发一枚 1 小时有效的 HS256 JWT
（见 `backend/internal/llm/auth.go`）。其他 OpenAI 兼容网关仍走静态 Bearer，行为不变。
`glm-5.3-flash` 是推理模型，流式响应前段只有 `reasoning_content`，客户端只取 `content`，
因此回答出现前会有一段静默，属正常现象。

继续使用现有嵌入配置时，升级后可直接使用 Qdrant 集合 `team_campusclaw_chunks` 中的存量索引。
**更换嵌入模型后必须全量重索引**，即便新旧模型维度相同。按原部署流程先删除该集合，
再以各班教师身份执行 `node scripts/reindex-all.mjs --reset`。

`/health` 的 `llm.enabled` 表示已配置模型。运行 `node scripts/verify-llm.mjs` 验证真实网关、
索引、向量召回和对话流，预期脚本退出码为 0。

**上传与重索引是异步的**（spec `async-indexing` / 第 8 课）

- `POST /api/materials` 与 `POST /api/materials/{id}/reindex` 都返回 **202**，
  响应体只有 `task_id` / `material_id` / `status` / `status_url`，不含索引结果。
- 材料与 `pending` 任务在同一个 MySQL 事务里写入（事务性发件箱）；同一班同一份内容
  重复提交由唯一约束复用同一任务，不产生第二份结果。
- `GET /api/tasks/{task_id}` 只读且按班隔离，返回 `pending → running → succeeded / failed / dead`
  以及 `attempts`、`error`。连续查询不会触发执行，跨班返回 404。
- 建索引由独立的 `worker` 服务执行：`SKIP LOCKED` 领取、租约到期回收、失败分类
  （凭证无效 / 额度耗尽直接终态，其余指数退避加抖动）、次数上限进死信；
  切片唯一约束与确定性 Qdrant 点 ID 保证重复投递不产生重复数据。
- 前端上传后轮询任务到终态再刷新列表；列表里还有 `pending` 材料时每 1.5 秒静默刷新。
- 判据复现：`docker compose stop worker` 后上传，任务停在 `pending`；
  `docker compose start worker` 后自动跑完。命令与输出见 `scripts/async-criteria.mjs` 与 `reports/`。

**历史问答与引用全文**（spec `ask-history` / `knowledge-retrieval` R7）

- 完整回答落库到 `conversations`（问题、回答、引用清单、时间）；客户端中断、空回答与未命中材料不落库。
- `GET /api/conversations?limit=20` 只读**本人 + 本班**记录，按 `id` 倒序，上限 50；未认证返回 401，`limit` 非法返回 400。
- SSE 的 `done` 事件带 `conversation_id`（落库失败为 `null`）；提问页「历史问答」面板可展开完整回答与当时的引用。
- 引用条目的「阅读全文」调用 `GET /api/materials/{id}` 打开**整篇材料原文**，并在正文中高亮定位命中切片；材料页与提问页共用同一套阅读面板（`Esc` 关闭、焦点归还）。
- 定向验收：`node scripts/verify-history-ui.mjs`，用真实存量数据验证，**不上传材料、不产生测试文本**。

**上传与输入边界**（spec R3.4 / R11）

- 正文上限 32 MiB；超过返回 413 `payload_too_large`（上传接口总请求上限 33 MiB）。
- 标题去空白后按字符计最多 255 个；超过返回 400 `title_too_long`，为空回退「未命名材料」。
- 只接受 `.txt / .md / .markdown / .pdf`；其余返回 415 `unsupported_file_type`。
- 文本正文必须是合法 UTF-8，否则 400 `invalid_encoding`；PDF 抽不出文本时 400 `empty_content`。
- 普通 JSON 接口只接受 `application/json`（否则 415），请求体上限 1 MiB（否则 413），
  且只允许单个顶层 JSON 值（尾随内容返回 400）。
- 材料正文列已迁移为 `LONGTEXT`；17 MiB 文本可正常落库并返回 202（建索引由 worker 异步完成）。

**演示语料（可选）**

`seed-data/biology/` 提供一批生物学材料（9 个领域，98 条），用于填充知识库做演示与检索测试。
仓库不保存 MySQL 数据卷，材料由下面的脚本从 `corpus.json` 重建（正文共约 20 万字、0.5 MB）：

```bash
cd seed-data/biology
node seed.mjs              # 以 teacher1 身份灌入 1 班，并等索引跑完
node seed.mjs --no-wait    # 只提交，不等索引完成
```

脚本按上传接口的异步契约工作：`POST /api/materials` 返回 **202 + task_id**，随后轮询
`status_url` 到 `succeeded`（`succeeded / failed / dead` 为终态）。因此**必须让 worker 在跑**
（`docker compose ps worker`），否则任务会停在 `pending`，脚本会超时并提示。

幂等且可自愈：标题已存在且可检索的跳过；已存在但未 ready 的提交重索引；不存在的才新建。
所以额度用尽（429）中断后直接重跑即可，不会重复建材料、也不会把切片翻倍。

未配置模型网关时，脚本仍然成功：材料会落库、切片会由 worker 建好（`qa.Index` 在
`llm.Enabled()` 为假时只做词面索引并返回），此时检索走 BM25 词面路、问答降级为抽取式回答。

材料由 `fetch-from-wikipedia.mjs` 从中文维基百科抓取生成（内容遵循维基百科的 CC BY-SA 授权），
已按约 2400 字切成大小适中的材料；重新抓取需能访问维基的网络（`PROXY=http://127.0.0.1:<端口> node fetch-from-wikipedia.mjs`）。

**操作系统课程讲义**

`seed-data/operating-systems/` 提供一班教师使用的六份中文讲义。运行 `node seed-data/operating-systems/seed.mjs --check` 检查，再运行 `node seed-data/operating-systems/seed.mjs` 导入。重复导入跳过相同正文，拒绝学生、跨班账号及同标题不同正文；来源和授权单独记录。详见该目录 README。

**只跑后端（本地开发，不需要 Docker）**

```bash
cd backend
export HTTP_ADDR=:8080 MYSQL_HOST=127.0.0.1 MYSQL_PORT=13306 \
       MYSQL_USER=campusclaw MYSQL_PASSWORD='<你的口令>' MYSQL_DATABASE=campusclaw \
       QDRANT_URL=http://127.0.0.1:6333 JWT_SECRET="$(openssl rand -hex 32)"
go run ./cmd/server
```

默认 `REQUIRE_DEPS=true`：依赖未就绪时**启动即失败退出**并打明确日志。
本地无依赖调试可加 `REQUIRE_DEPS=false`，服务照常启动，此时 `/health` 返回 **503** 并逐项说明哪个依赖不可用。

**质量门禁（提交前必跑）**

```bash
node scripts/quality-gate.mjs
# Go 不在 PATH 时：
GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs
```

门禁覆盖：配置样例完整性、分层依赖、密钥卫生、gofmt、`go build`、`go vet`、`go test`。
加 `--with-spec` 追加 OpenSpec 严格校验，加 `--with-e2e` 追加接口端到端（需服务已启动）。

**端到端验收（退出码即判定）**

```bash
# 后端（需服务在跑；直连数据库核对，故需提供 MYSQL_*）
cd backend
MYSQL_HOST=127.0.0.1 MYSQL_PORT=13306 MYSQL_USER=campusclaw MYSQL_PASSWORD='<口令>' \
  go run ./cmd/e2e            # 全绿退出码 0

# 前端（真实 Chrome + CDP）
node scripts/e2e-ui.mjs       # 全绿退出码 0
```

## 工程规约与协作

| 入口 | 内容 |
|------|------|
| `AGENTS.md` | Harness 与人类共同遵守的工程规约（变更流程、分层、配置、日志、安全、测试、DoD） |
| `.qoder/rules/` | 八类规范细则：产品设计 / 后端 / 安全 / 测试 / 部署运维 / AI 应用 / 数据 / 评审完成 |
| `.qoder/skills/` | 可复用流程：新增后端接口、运行质量门禁、从零启动 |
| `scripts/quality-gate.mjs` | 一条命令跑完离线门禁 |
| `docs/lesson-07/` | 第七课骨架验收记录与同伴评审表 |

规范不是建议：每条规则都写明检查方式、违规处理与有效性验证；故意违规演练记录在 `docs/lesson-07/verification.md`。

## 仓库当前状态

- 第一轮变更 `add-core-qa-loop`（登录 → 上传 → 问答 → 引用的垂直切片）**已落地并验收**：
  规约 `validate --all --strict` 通过；后端 41 项、前端 13 项端到端用例全绿；Compose 一键启动实测通过。
- 第七课基线变更 `2026-10-28-lesson-07-baseline`：补齐规约前置条件、八类规范与三个技能，
  硬化骨架（配置失败即退、JSON 日志 + 请求标识、`/health` 含版本与时间、分层与密钥门禁）。
  验收状态见 [`docs/lesson-07/verification.md`](./docs/lesson-07/verification.md)；同伴评审表见
  [`docs/lesson-07/peer-review.md`](./docs/lesson-07/peer-review.md)（尚未由第二位成员签署）。
  基线标签为 `v0.0.1-skeleton`；`main` 已启用保护（PR + 1 人批准、线性历史、禁强推/删除），日常改动走分支与 PR。
- 2026-10-31 起的三个活动 change（学习工作台重设计、Go 换行治理、材料全文阅读）已实现并通过当前工作区验收：
  完整质量门禁全绿，Go 接口 e2e 72 项、浏览器 e2e 50 项通过。`GET /api/materials/{id}` 为本班教师与学生
  返回完整原文，跨班与不存在统一 404；三者尚未完成同伴评审与归档，状态以对应 change 与 PR 为准。
- 选题说明（完整版，含表格与排期）：[`CampusClaw_课程项目选题说明_修订版.pdf`](./CampusClaw_课程项目选题说明_修订版.pdf)

## 文档

| 文档 | 说明 |
|------|------|
| `CampusClaw_课程项目选题说明_修订版.pdf` | 项目选题说明（题目、功能、技术方案、分工） |
| `AGENTS.md` | 工程规约总入口 |
| `openspec/` | 规约驱动骨架；各轮变更见 `openspec/changes/` |
| `docs/lesson-07/verification.md` | 第七课五维检查表与实测证据 |
| `docs/lesson-07/peer-review.md` | 同伴评审记录（待签） |

## 许可证

本仓库为北京大学「互联网软件开发技术与实践」课程小组作业，**未授予开源许可**；未经作者书面同意，请勿复制、分发或用于其他项目。第三方内容（演示语料）遵循其原始授权（CC BY-SA）。

上传事务回滚测试使用独立 MySQL 8.4，PR 的 `check` 作业会自动执行。本地复跑的环境变量、命令和判据见[事务风险核查](reports/risk-transaction.md)。

问答压测会分别统计完整回答、正常拒答与错误，并输出首字和完成时延。验收口径及单请求复跑命令见[压测报告](reports/bench.md)。

同一材料并发上传的验收命令、环境前置和实际数据库计数见[并发风险核查](reports/risk-concurrency.md)。
