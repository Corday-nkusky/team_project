# Design

## Context

- 第 7 课要求：建仓库 → 定规约 → 立规范 → 落骨架；五维核查覆盖仓库、规约、规范、骨架、交付。
- 本仓已有第一轮垂直切片（登录 / 上传 / 问答 / 引用），业务行为可用；缺的是可被判定的基线。
- 约束：不改业务行为；本地零密钥仍可跑通（`LLM_API_KEY` 为空时走抽取式回答）；SSE 不能被日志中间件破坏。

## Goals / Non-Goals

**Goals:**

- 让「配置缺失」「日志不可串联」「健康检查不可探活」都变成可判定的失败，而不是口头约定。
- 把五类工程规范放到 Harness 每次改代码都会读到的位置，并给出检查方式与违反处理。
- 用一条可复现的质量门禁守住分层与密钥边界。
- 留下 `docs/lesson-07/` 验证记录和 `v0.0.1-skeleton` 标签，作为后续课次的回归对照点。

**Non-Goals:**

- 不新增业务功能；不重构认证、检索、问答实现。
- 不重写 Git 历史；首提交时序作为历史缺口记录，不通过改写掩盖。
- 不引入新的日志库、配置库或测试框架；保持标准库与现有依赖。

## Decisions

### D1 配置分级：部署关键项必需，调优项允许显式默认

`HTTP_ADDR`、`MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_DATABASE`、`QDRANT_URL`、`JWT_SECRET` 为必需项；缺失、为空或格式非法时，服务在监听前以非零码退出，并输出 JSON 日志指明键名。`LLM_API_KEY` 允许为空（显式关闭模型），此时 `LLM_API_BASE`、模型名不必填；填了 key 则 `LLM_API_BASE` 必须填。

调优项（TTL、切片大小、TopK、阈值、等待时长）保留默认值，但全部可在 `.env` 覆盖；默认值属于公开的调优参数，不是密钥或地址的静默降级。

被放弃的候选方案：

- **所有 env 一律必需**：本地 e2e 与调试需要设置十几个变量，容易促使成员把值写死回代码；只把会影响安全与连通性的项设为必需。
- **继续保留默认值**：无法区分「忘了配」与「配成了默认值」，与本课「缺了就启动失败」直接冲突。

### D2 日志：标准库 `slog` JSON + 自生成 request ID

使用 Go 标准库 `log/slog` 的 `NewJSONHandler`，运行期日志统一走 `slog`；每个 HTTP 请求生成或透传 `X-Request-ID`，响应头回写，访问日志只打一条，字段为 `request_id`、`method`、`path`、`status`、`duration_ms`。客户端提供的 ID 只接受 1–128 位可打印 ASCII，否则重新生成；request ID 仅用于关联，不参与授权。

被放弃的候选方案：

- **引入 zap / zerolog**：多一个依赖，标准库已满足「单行 JSON + 字段」。
- **沿用 `log.Printf`**：无法稳定解析，也无法把请求链路串起来。

### D3 健康检查：version / time / deps 同体返回

`GET /health` 保持免认证；依赖全好返回 200，任一不可用返回 503。响应含 `status`、`version`、`time`（UTC RFC3339）、`deps`（mysql / qdrant）与 `llm.enabled`。`version` 由构建参数注入，默认 `dev`；Dockerfile 接收 `VERSION` build arg。

被放弃的候选方案：

- **只返回 `{"status":"ok"}`**：无法判断部署的是哪个版本，也无法在故障时对齐时间。
- **把健康检查放进认证中间件**：编排无法探活，违背 readiness probe 的用途。

### D4 分层：单向依赖 + 显式白名单例外

目标依赖方向：`cmd`（装配）→ `httpapi`（接口）→ `qa`（业务）→ `store` / `retrieval` / `llm` / `qdrant`（数据与适配器）。允许 `httpapi` 直接调用 `store`，但**仅限门禁脚本中的白名单文件**，且只做单表读写与会话记录落库：`auth.go`（账号与令牌）、`materials.go`（材料元信息）、`ask.go`（会话记录）、`router.go`（类型装配）。检索、生成、切片等跨聚合流程必须经 `qa`。白名单外新增 handler→store 依赖会被门禁拒绝。禁止任何 `store`、`qa`、适配器反向 import `httpapi`。

`scripts/check-layering.mjs` 读取 Go import 图，发现禁止边即非零退出；规范里写明这条例外，避免「默许」变成隐性规则。

被放弃的候选方案：

- **立即把全部 handler 重构为 service**：会把基线变更扩大成业务重构，风险高于收益；先用门禁锁住方向，后续按增量收敛。
- **不设例外、一律禁止 handler→store**：与现有简单 CRUD 冲突，且课程允许把有理由的旧写法写进规范。

### D5 规范落点：AGENTS.md + rules + skills

常驻约束写进根 `AGENTS.md` 与 `.qoder/rules/*.md`；按场景执行的流程写进 `.qoder/skills/*/SKILL.md`。五类规范各一份文件，另补 AI、数据、评审与完成定义三类。每条规范包含：约束、检查方式、违反处理；能用脚本检查的写进 `scripts/quality-gate.mjs`。

被放弃的候选方案：

- **只写一份大而全的规范**：Harness 每次读全文，成本高且难定位；按类别拆文件，AGENTS.md 只做索引与硬约束。
- **只写在会话/记忆里**：换人或换会话即失效，不满足「Harness 读得到」。

### D6 交付：验证记录 + 标签

`docs/lesson-07/verification.md` 记录实际命令与输出；`docs/lesson-07/peer-review.md` 是交叉评审模板，由组内另一名成员在自己机器上执行后填写，未执行前保持未勾选，不代填。归档本变更后，在通过门禁的提交上打 `v0.0.1-skeleton`。

被放弃的候选方案：

- **给旧骨架提交 `176fa01` 直接打标签**：该提交的配置默认值、文本日志与 `/health` 字段不满足本课判据，标签会误导后续回归。
- **只写 README 不建 docs**：课程明确要求验证记录归档到仓库 `docs/`。

## Risks / Trade-offs

- **配置变严格会让旧命令失败**：README、`.env.example`、Compose 同步更新；本地 `.env` 需要填一个非示例 `JWT_SECRET`。
- **日志中间件包 ResponseWriter 可能破坏 SSE**：`statusRecorder` 继续透传 `Flush()` 与 `Unwrap()`；用现有 Go e2e 的流式用例复验。
- **request ID 透传客户端值可能被伪造**：限制长度与字符集，只用于日志关联，不参与授权判定。
- **只读 CRUD 例外可能被滥用**：门禁只放行 `httpapi → store`，并禁止 `store → httpapi`；规范限定为单表查询。
- **规范文件多，维护成本上升**：AGENTS.md 做索引，规则按类别拆分；质量门禁只强制能自动判定的部分，人工项进入评审表。
