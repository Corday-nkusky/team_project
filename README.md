# CampusClaw 智教闭环

> 基于 RAG 与教学智能体的课堂疑问收集、个性化复习及教学反馈系统
> —— 北京大学「互联网软件开发技术与实践」课程小组项目

## 项目定位

面向**班级教学场景**，本仓**从零构建**一套完整系统，**不基于任何既有系统做增量**：

- **底座**（本轮起逐步自建）：教师上传材料、班级权限隔离、文本切片、混合检索、带引用的 RAG 问答、SSE 流式回答。
- **目标**：把「学生即时提问 — 持续复习 — 教师反馈」打通成**可追踪的教学闭环**。

> 说明：课程中出现的 CampusClaw 演示系统**只作问题域参考**（哪些坑踩过、哪些约束成立），
> **不复制其代码与结构**；本仓的认证、权限、检索等一律自行实现。

项目采用**真实业务数据与可解释的统计指标**，不把「问答次数」直接等同于「学习效果」。

## 核心功能模块

| 模块 | 做什么 |
|------|--------|
| **课堂疑问收集** | 学生在阅读材料或使用 AI 解题助手时，对知识点、材料片段或某次回答提交疑问；支持「仅教师可见 / 班级匿名展示」、疑问状态、相似问题合并、教师统一答疑；保留原文引用与会话来源。 |
| **个人学习路径** | 学生按知识点标记「已掌握 / 需要复习 / 仍有疑问」；关联材料片段、历史问答与疑问记录，自动生成待复习清单；按复习间隔与学习状态发出回顾提醒，允许手动调整掌握状态。 |
| **教学反馈看板** | 教师查看本班知识点疑问热度、未解决问题、材料引用频次与阶段变化；按知识点聚合共性问题，定位需补充讲解的内容；只看班级汇总，不向其他学生公开个人学习记录。 |

### 教学闭环流程

```
教师上传并组织班级材料
   → 学生使用智能体进行有引用的学习问答
   → 学生提交疑问并标记掌握状态
   → 系统将疑问和复习任务关联到知识点
   → 教师查看班级聚合反馈并集中答疑
   → 学生回顾复习、更新状态
```

## 技术方案

**系统架构**：React + TypeScript 前端 / Go REST 与 SSE API / MySQL 8.4 业务数据 / Qdrant 向量检索 / OpenAI 兼容模型网关 / Docker Compose。Web 与索引 worker 是两个可独立启停的进程，共用同一镜像与外置配置。前端产物由 Go `embed` 内嵌，与接口**同源单端口**，因此没有独立反向代理。认证、教师与学生 RBAC 及班级隔离机制**由本仓自行实现**。

| 模块 | 技术栈 | 实现方案 | 验收目标 |
|------|--------|----------|----------|
| 课堂疑问收集 | Go、MySQL、Qdrant、JWT | 建立疑问记录、状态及材料/知识点关联；以检索召回相似疑问，由教师确认合并；支持匿名班级展示与统一答疑。 | 学生可提交与追踪疑问；教师可处理共性问题；跨班数据不可见。 |
| 个人学习路径 | React、TypeScript、Go、MySQL | 按知识点保存掌握状态、关联材料与历史提问；依据状态和最近复习时间生成可解释的复习任务与提醒。 | 个人复习清单可持久化、更新和追溯；他人不可访问。 |
| 教学反馈看板 | React 图表、Go、MySQL | 按班级、知识点、状态及时间聚合疑问；展示高频知识点、未解决率、引用材料与变化趋势，支持过滤及查看来源。 | 教师可查看本班指标和原始记录，学生个人明细不被泄露。 |
| 智能体与 RAG 集成 | Go、Qdrant、SSE、LLM API | 自带 JWT 权限、混合检索、技能路由、多轮会话及流式回答；在引用位置添加「提交疑问」及「加入复习」入口。 | 新模块能关联真实材料和引用；检索、问答及 SSE 可用。 |
| 测试与部署 | Go 测试、Node 脚本、Docker Compose | 维护 Go 单元测试与接口/浏览器端到端脚本；质量门禁（分层、密钥、格式、构建、vet、单测）一条命令；权限负向用例与持久化校验纳入 e2e。 | 一键部署可用；门禁与 e2e 以退出码判定，结果可复现。 |

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
| 业务状态 | — | 第 2 轮：疑问记录与状态流转 |

**B. 工程与交付要求**（不是功能，是"怎么做到"与交付约束）

| 要求 | 落点 |
|---|---|
| 测试 | 第 1 轮：端到端验收脚本（含权限负向用例）；此后持续补用例 |
| 审计 | 第 1 轮：登录 / 上传 / 问答留痕；后续补完整审计（防篡改、查询界面） |
| 部署 | 第 1 轮：Compose 一键启动 + 探活 |
| 异步任务 | **不单列为功能**，而是「上传材料」的实现方式：上传返回 202 + 任务标识，MySQL 任务表作持久化队列，独立 worker 异步建索引（第 8 课要求，已落地） |
| 公网可访问 | **待定**：本版上线，还是留到下一轮 |
| 规约驱动 | 第 1 轮起：`openspec/` 四件套，每轮持续 |

**C. 五项 AI 能力**

| 能力 | 第 1 轮 | 后续轮次（草排） |
|---|---|---|
| LLM | ✅ 据材料生成回答 | |
| 知识库与 RAG | ✅ 本班材料检索 + 引用回看 | 第 2 轮起：重排 / 查询改写 |
| 智能体 | — | 第 2 轮：疑问相似合并 |
| 技能 | — | 第 2 轮：技能文件与路由 |
| MCP | — | 第 3 轮：工具层与工具级运行时授权 |

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

脚本会生成 `deploy/.env`（首次自动填入随机密钥）、构建镜像、启动容器，并轮询
`http://127.0.0.1:8080/health`；就绪后打印 `version`、`time` 与依赖状态。
Compose 起四个服务：`mysql` / `qdrant` / `backend`（Web）/ `worker`（索引消费者），
后两者共用同一镜像与配置，只是入口不同。
浏览器打开 `http://127.0.0.1:8080` 即为应用（前端由 Go 内嵌，同源单端口）。

手动方式（等价）：

```bash
cd deploy
cp .env.example .env        # 必须把 JWT_SECRET 换成随机长串：openssl rand -hex 32
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

**更换嵌入模型后必须全量重索引**：即使维度相同（`course-embedding` 与 `embedding-3` 同为
2048 维），向量空间不同也不能新旧混用。先删除 Qdrant 集合 `team_campusclaw_chunks`
（清掉旧向量与已删材料的孤儿点），再执行 `node scripts/reindex-all.mjs --reset`；
脚本按 202 + 轮询逐篇重建，遇到 429 / 额度耗尽立即停并保存进度。

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

`seed-data/` 下有两批语料，均抓自中文维基百科条目正文（CC BY-SA），不是游戏或教材原文：

| 目录 | 内容 | 规模 |
| --- | --- | --- |
| `biology/` | 生物学（9 个领域） | 98 条材料，约 20 万字 |
| `genshin/` | 《原神》10 位角色条目 | 18 条材料，约 4 万字 |

仓库不保存 MySQL 数据卷，材料由脚本从各目录的 `corpus.json` 重建：

```bash
cd seed-data
node seed.mjs                     # 默认 biology：以 teacher1 身份灌入 1 班，并等索引跑完
node seed.mjs --corpus genshin    # 换成《原神》角色语料
node seed.mjs --no-wait           # 只提交，不等索引完成
```

标题形如 `生物化学｜酶（1/3）`、`角色设定｜钟离（1/2）`，前一段是领域，脚本按标题幂等，
所以两批语料可以灌进同一个班做跨领域检索演示（问生物化学命中生物材料，问角色命中角色材料）。

脚本按上传接口的异步契约工作：`POST /api/materials` 返回 **202 + task_id**，随后轮询
`status_url` 到 `succeeded`（`succeeded / failed / dead` 为终态）。因此**必须让 worker 在跑**
（`docker compose ps worker`），否则任务会停在 `pending`，脚本会超时并提示。

幂等且可自愈：标题已存在且可检索的跳过；已存在但未 ready 的提交重索引；不存在的才新建。
所以额度用尽（429）中断后直接重跑即可，不会重复建材料、也不会把切片翻倍。

未配置模型网关时，脚本仍然成功：材料会落库、切片会由 worker 建好（`qa.Index` 在
`llm.Enabled()` 为假时只做词面索引并返回），此时检索走 BM25 词面路、问答降级为抽取式回答。

材料由 `fetch-from-wikipedia.mjs` 从中文维基百科抓取生成（内容遵循维基百科的 CC BY-SA 授权），
已按约 2400 字切成大小适中的材料；重新抓取需能访问维基的网络（`PROXY=http://127.0.0.1:<端口> node fetch-from-wikipedia.mjs`）。

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
