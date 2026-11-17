# AGENTS.md — CampusClaw 工程规约

本文件是 Harness（Claude Code / Qoder / Cursor 等）与人类成员共同遵守的唯一入口。
细则在 `.qoder/rules/`，可复用流程在 `.qoder/skills/`。**规范不是建议**：每条规则都写明检查方式与违规处理。

## 项目一句话

教师上传教学材料，学生在**本班范围**内提问，得到带原文引用的流式回答；材料中没有依据时如实回答「资料中未找到相关内容」。

## 一、变更流程（不可跳过）

1. 先写 OpenSpec change：`openspec/changes/YYYY-MM-DD-<动词>-<名词>/`，四件套 `proposal.md` / `design.md` / `specs/<capability>/spec.md` / `tasks.md`。
2. 每个 Scenario 必须三段式：**GIVEN**（前置条件）/ **WHEN**（操作）/ **THEN**（可观测结果）。
3. 每个 Requirement 至少一条失败路径；每份 tasks 的每条任务都要有可判定的 `verify:`。
4. 实现完成后把**实测**写进对应任务（命令 + 结果），再勾选。
5. 归档：`openspec archive`，主规约同步；变更目录移入 `changes/archive/`。
6. 提交信息引用 change 名或 Requirement 编号（例：`feat(R8): 配置失败即退`）。

## 一之一、AI 生成代码核查清单（七类，写进规则才生效）

> 课程「下一阶段重点」要求：把七类核查对象写进 Harness 会读取的规则文件，每类写明**检查手段**。
> 审核任何 AI 原生代码时逐类过，答不上来就停下来改；不能只看"能不能跑"。

| 类别 | 具体要问的问题 | 检查手段 |
| --- | --- | --- |
| 接口入口 | 参数都校验了吗？除"登录了没有"有没有检查数据属于当前用户？返回有没有多余字段？ | `go test ./internal/httpapi` 负向用例；e2e 401/403/404；Swagger 对照 |
| 业务逻辑 | 与规约场景逐条对过吗？状态流转有无漏中间态？有没有违反本组规范？ | 可追溯矩阵（`docs/traceability.md`）；`openspec validate --strict` |
| 数据库 | 连续写多处是否同事务？唯一性靠数据库约束还是"先查一遍"？ | 表 DDL 的 `UNIQUE KEY` / 事务代码走查；`reports/risk-*.md` |
| 公共函数 | 同一段逻辑是否复制到多个文件？出错是吞掉还是明确报出？ | `grep` 重复片段；`go vet`；代码走查 |
| 错误处理 | 每种失败都有出口吗？报错是否暴露内部结构/路径/配置？ | 单测失败分支；`writeErr` 统一错误体；日志脱敏检查 |
| 安全与隐私 | 敏感字段脱敏了吗？用户输入先校验再用了吗？输出转义了吗？ | `make check-security`；`check-secrets.mjs`；注入反例用例 |
| 性能 | 循环里逐条查库了吗？查询条件有索引吗？列表有分页吗？ | `reports/bench.md`；慢查询走查；压测脚本 |

**三分法结论**：`可用`（逻辑正确、符合规约与安全要求 → 补测试、交评审）/ `待修改`（方向对但有缺陷 → 列出修改项、改完重审）/ `风险`（越权、数据错乱或与需求无关 → 丢弃重写）。
**高风险三查**（并发 / 事务 / 权限）必须有专门记录，不允许跳过：`reports/risk-concurrency.md`、`reports/risk-transaction.md`、`reports/risk-authz.md`。
**准入流程**：AI 初稿 → 自己逐段审核 → 交叉评审 → 合入 main；任何合入 main 的变更都要有对应审核记录。

## 二、分层（门禁：`node scripts/check-layering.mjs`）

| 层 | 包 | 允许依赖 |
| --- | --- | --- |
| 3 入口 | `cmd/server`、`cmd/e2e` | 任意内部包 |
| 2 传输 | `internal/httpapi` | qa、store（白名单文件）、config/auth/deps/web |
| 1 应用 | `internal/qa` | store、retrieval、llm、qdrant |
| 0 基础 | config/auth/db/deps/llm/qdrant/retrieval/store/web | 仅标准库与本层 |

禁止：反向依赖；`httpapi` 直接 import `llm`/`qdrant`/`retrieval`；`web` 依赖任何内部包；新增内部包未在门禁脚本登记层级。

## 三、配置（spec R8）

- 八个必填项：`HTTP_ADDR`、`MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_DATABASE`、`QDRANT_URL`、`JWT_SECRET`。
- 代码不保留可用默认值；缺失、为空或非法 → **监听端口前**非零退出，JSON 日志只报键名、不回显值。
- 调优项可有默认值；一旦显式设置就必须能解析，解析失败同样报错。
- 模型网关可选：`LLM_API_KEY` 为空即抽取式降级；一旦设置，`LLM_API_BASE`、`LLM_CHAT_MODEL`、`LLM_EMBED_MODEL` 必须同时提供，且网关须同时支持对话 SSE 与 `/embeddings`。模型名不设默认值（spec LG1）。
- 密钥只进本机 `deploy/.env`（git 忽略）；`.env.example` 只放占位符。

检查：`node scripts/check-secrets.mjs`、`go test ./internal/config`、新环境 `node scripts/up.mjs`。
违规处理：CI/门禁失败即阻断合并；已合入的按 P0 处理，下一提交修复并补测试。

## 四、日志（spec R10）

- 单行 JSON 到 stdout；每条请求一条访问日志，字段含 `request_id`、`method`、`path`、`status`、`duration_ms`。
- 禁止记录 `Authorization`、请求体、查询串、口令、令牌。
- 客户端 `X-Request-ID` 仅在长度 1–128 且全部可打印 ASCII 时透传，否则重新生成并回写响应头。

检查：`go test ./internal/httpapi`（日志字段与 ID 透传）、e2e「R10 请求标识」。
违规处理：泄漏类问题视为安全事故，立即回滚该提交并轮换受影响密钥。

## 五、接口与数据

- 错误体统一 `{"error":{"code":string,"message":string}}`；400 参数、401 未认证、403 越权、404 不存在、503 依赖不可用。
- 任何业务查询必须带 `class_id` 约束；列表排序必须含唯一 tiebreaker（如 `id DESC`）。
- 新接口必须同时具备：路由、鉴权/角色、审计（若属登录/上传/问答）、e2e 断言、文档更新。
- 数据库变更只能向前兼容：先加列/表，再切读写，最后清理；禁止破坏性改列。

## 六、安全

- 口令只存 bcrypt；refresh token 只存 SHA-256 摘要；任何日志与响应不含明文。
- 越权默认拒绝；跨班请求返回 403/404，且不泄漏资源存在性。
- 上传限制 32 MiB，仅接受纯文本 / Markdown / PDF 文本。
- `/health` 免认证，但不得包含密钥、口令或连接串。

## 七、测试与门禁

- 每个 Scenario 至少一条自动化断言；负向用例不少于正向的一半。
- 一条命令：`node scripts/quality-gate.mjs`（配置样例、分层、密钥、gofmt、build、vet、test）。
- 端到端：`GO_BIN=... node scripts/quality-gate.mjs --with-e2e`；以退出码判定。
- 测试数据用随机独有词隔离，禁止依赖执行顺序或历史数据。

## 八、完成定义（DoD）

代码 + 测试 + 文档 + 门禁全绿 + tasks 勾选并写实测 + 主规约与实现一致 + **另一位成员**按 `docs/lesson-07/peer-review.md` 完成评审。自审不能替代同伴评审。

## 九、规范索引

| 规范 | 文件 | 主要检查方式 |
| --- | --- | --- |
| 产品设计 | `.qoder/rules/01-product-design.md` | OpenSpec 严格校验 + 评审清单 |
| 后端开发 | `.qoder/rules/02-backend.md` | `check-layering.mjs` + `go vet` |
| 安全开发 | `.qoder/rules/03-security.md` | `check-secrets.mjs` + e2e 负向用例 |
| 测试 | `.qoder/rules/04-testing.md` | `quality-gate.mjs` + e2e 退出码 |
| 部署运维 | `.qoder/rules/05-deploy-ops.md` | `up.mjs` + `/health` |
| AI 应用 | `.qoder/rules/06-ai-application.md` | e2e「R5.2 未命中」断言 |
| 数据 | `.qoder/rules/07-data.md` | 迁移评审 + 索引状态断言 |
| 评审与完成 | `.qoder/rules/08-review-and-done.md` | `peer-review.md` 签字 |

## 十、规范有效性验证

每次修改规范后做一次「故意违规」演练，证明门禁真的会拦：例如在 `httpapi` 里 import `qdrant`、在 compose 给 `JWT_SECRET` 加 `:-` 回落、提交一个未格式化的 Go 文件，确认对应门禁失败并记录到 `docs/lesson-07/verification.md`。
