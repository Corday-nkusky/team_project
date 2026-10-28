# Tasks

> 按编号顺序落地；每条以可判定的 `verify:` 说明「怎么算完成」。完成一条勾一条。
> 本次只补第 7 课基线，不新增业务能力；骨架任务排在最前。
>
> 验收证据详见 `docs/lesson-07/verification.md`。

## 1. 骨架与运行基线

- [x] 1.1 配置分级：部署关键项必需，缺失/非法即启动失败
  - verify: 清空 `JWT_SECRET` 启动 → 非零退出且 JSON 日志含 `JWT_SECRET`（不含值）；`MYSQL_PORT=abc` → 非零退出
  - 实测：`go test ./internal/config` 9 个用例通过；编译二进制实测 4 条失败路径（缺 JWT / 示例密钥 / 端口越界 / 依赖未就绪），均单行 JSON、退出码 1、只报键名。
- [x] 1.2 运行日志改为单行 JSON，并带 request ID
  - verify: 请求带 `X-Request-ID: peer-123`，响应头回写同值；标准输出新增一行可 `JSON.parse` 的日志，含 `request_id/method/path/status/duration_ms`
  - 实测：`go test ./internal/httpapi -run TestRequestIDAndAccessLog`；`peer-123` 原样回写，日志单行可解析且五字段齐全；非法 ID 被替换为 32 位十六进制。
- [x] 1.3 `/health` 增加 version 与 time
  - verify: 不带令牌请求 `GET /health`，返回含 `version`、`time`、`deps`；依赖不可用时为 503
  - 实测：`TestHealthOK` / `TestHealthDegraded` 通过；200 含 RFC3339 `time` 与 `version=test-version`，依赖失败时 503 且另一依赖仍报 `ok`。
- [x] 1.4 分层与密钥门禁脚本
  - verify: `node scripts/check-layering.mjs` 与 `node scripts/check-secrets.mjs` 在干净树退出 0；引入禁止 import 或密钥样例时退出非零
  - 实测：干净树均 exit 0；演练（httpapi→qdrant、compose `:-` 回落）分别 exit 1，详见 verification.md「四」。
- [x] 1.5 Compose / Dockerfile / `.env.example` / README 同步
  - verify: 从零复制 `.env.example` 为 `.env` 并填 `JWT_SECRET` 后，一条命令启动，`/health` 成功
  - 实测：`node scripts/up.mjs` 构建并启动三容器，`/health` 200 `deps={"mysql":"ok","qdrant":"ok"}`；`VERSION=v0.0.1-skeleton` 重建后 `/health` 返回同值版本。Compose 必填 `${VAR:?}`、Dockerfile 注入版本、`.env.example` 占位符、README 已同步。

## 2. 规范与技能

- [x] 2.1 根 `AGENTS.md`：项目上下文、硬约束、命令与规范索引
  - verify: 新会话只读仓库即可找到五类规范、技能与质量门禁
  - 实测：`AGENTS.md` 十节；「规范索引」列出八类规则文件与三个技能；硬约束给出门禁命令与违规处理。
- [x] 2.2 五类规范文件（产品 / 后端 / 安全 / 测试 / 部署运维）
  - verify: 每类含约束、检查方式、违反处理；文件位于 `.qoder/rules/`
  - 实测：`01-product-design.md`–`05-deploy-ops.md`，每份均含「规则 / 检查方式 / 违规处理 / 有效性验证」。
- [x] 2.3 三类补充规范（AI 应用 / 数据 / 评审与完成定义）
  - verify: 与项目实际（LLM + Qdrant + MySQL）对应，不写空泛口号
  - 实测：`06-ai-application.md`（引文与无依据作答）、`07-data.md`（索引状态机、审计只增）、`08-review-and-done.md`（DoD 与评审清单）。
- [x] 2.4 按场景执行的技能（新增接口 / 补测试 / 从零启动）
  - verify: 每个 `SKILL.md` frontmatter 合法，正文给出可执行步骤
  - 实测：`add-backend-endpoint`、`run-quality-gate`、`zero-to-running`；frontmatter 名称与目录一致、描述单行 ≤1024 字符、正文 50–90 行。
- [x] 2.5 规范生效验证：按规范改一次代码，另起会话再验一次
  - verify: 新接口落在正确层、命名与错误格式符合规范；另起无上下文会话得到一致结论
  - 实测：本变更自身按规范实施（分层、`writeErr`、门禁）。另起**无上下文只读代理**回答「新增 `GET /api/materials/{id}` 按规范怎么做」，仅凭仓库文档即给出：四件套与 GIVEN/WHEN/THEN、handler 落 `httpapi` + store 单表读、门禁白名单、e2e 负向与 `quality-gate.mjs`，来源均指向 `AGENTS.md` 与对应 rules/skills——与规范一致。未真的新增接口（proposal 已列为非目标）。

## 3. 规约与交付证据

- [x] 3.1 既有主规约 24 个场景补「前置条件」，并补能力↔场景对照表
  - verify: 每条 Scenario 含 GIVEN/WHEN/THEN；对照表覆盖 `auth-upload`、`knowledge-retrieval`、`runtime-baseline`
  - 实测：`grep` 统计 24 Scenario / 24 GIVEN / 24 WHEN / 24 THEN；两份主规约末尾均有三能力对照表。
- [x] 3.2 `docs/lesson-07/verification.md` 记录实际命令与输出
  - verify: 记录能复现 `/health`、配置失败、JSON 日志、规约校验与门禁
  - 实测：五维检查表 + 四项实测输出 + 未执行项清单（Docker 与 e2e 在收尾时补测）。
- [x] 3.3 `docs/lesson-07/peer-review.md` 交叉评审模板
  - verify: 模板含成员、机器、步骤、失败点与修复；未执行前不勾选
  - 实测：模板含复现命令、15 项清单、意见表与签字栏；状态标注「未签署」，未代填。
- [x] 3.4 `openspec validate --all --strict` 通过
  - verify: 退出码 0
  - 实测：`Totals: 3 passed, 0 failed`。
- [x] 3.5 归档本变更并打 `v0.0.1-skeleton`
  - verify: `openspec/changes/archive/2026-10-28-lesson-07-baseline/` 存在；`git tag -l` 含该标签
  - 实测：归档目录存在；附注标签 `v0.0.1-skeleton` 打在 `c9a0ab0`，已推送 origin（远端标签对象 `563a9d9`，解引用为 `c9a0ab0`）。
- [x] 3.6 提交并推送；更新 README 的仓库状态
  - verify: `git status` 干净；`origin/main` 与本地一致
  - 实测：基线提交 `c9a0ab0` 推送到 `origin/main`；收尾文档提交后再次推送，`git status` 干净，`git ls-remote` 的 `refs/heads/main` 与本地 HEAD 一致。
