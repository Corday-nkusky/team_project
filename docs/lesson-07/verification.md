# 第七课骨架验收记录

> 对应课程任务：建仓库 → 定规约 → 立规范 → 落骨架。
> 本文件只记录**实际执行过**的检查；未执行项明确标注，不代填。

- 验收日期：2026-10-28
- 变更：`openspec/changes/2026-10-28-lesson-07-baseline/`
- 版本标签：`v0.0.1-skeleton`（打在骨架硬化提交上）
- 环境：Windows 11 + Git Bash；Go 1.27.1（`/d/tools/go/bin/go.exe`）；Node v24.21.0

## 一、五维检查表

### 1. 仓库

| 检查项 | 证据 | 状态 |
| --- | --- | --- |
| 公开仓库、三人协作 | GitHub `Corday-nkusky/team_project`；owner + 2 名 write 协作者 | ✅ |
| 仓库名符合「小写短名」 | 当前为 `team_project`（下划线） | ❌ 待改名，见「未完成」 |
| `main` 受保护、禁止直推 | `gh api .../branches/main/protection` 与 `rulesets` | ⏳ 本次收尾时配置 |
| LICENSE / 授权声明 | README「许可证」节 | ✅（保守声明，未授予开源许可） |
| 提交信息引用规约 | 见 `git log`，历史提交早于本规范 | ⚠️ 自本变更起执行 |

### 2. 规约（OpenSpec）

| 检查项 | 证据 | 状态 |
| --- | --- | --- |
| change 四件套齐全 | `proposal.md` / `design.md` / `specs/runtime-baseline/spec.md` / `tasks.md` | ✅ |
| change 名带日期 | `2026-10-28-lesson-07-baseline` | ✅ |
| 场景三段式 | 主规约 24 条 + 新增 16 条，均含 GIVEN/WHEN/THEN | ✅ |
| 失败路径 ≥ 4 | R1.2/R1.3/R1.7/R2.1–R2.3/R3.2/R3.3/R5.2/R5.3/R6.3/R8.2–R8.5/R9.2/R9.3/R10.3/R10.5 | ✅ |
| 能力↔场景对照 | proposal 与两份主规约末尾的对照表 | ✅ |
| 严格校验 | `npx @fission-ai/openspec@latest validate --all --strict` | ✅ 3 passed, 0 failed |
| 新会话可实施 | tasks 每条含 `verify:` 与落点文件 | ✅ |

主规约场景数：`auth-upload` 15 条（R1.1–R4.2）、`knowledge-retrieval` 9 条（R5.1–R7.3）；
本变更新增 `runtime-baseline` 16 条（R8.1–R10.5）。

### 3. 规范

| 检查项 | 证据 | 状态 |
| --- | --- | --- |
| Harness 可读取 | 根 `AGENTS.md`（总入口） | ✅ |
| 五类必备规范 | `.qoder/rules/01`–`05` | ✅ |
| 可选规范 | `.qoder/rules/06` AI 应用、`07` 数据、`08` 评审与完成 | ✅ |
| 每条规范含检查方式/违规处理/有效性验证 | 八个规则文件均有三节 | ✅ |
| 可执行载体 | `.qoder/skills/`：新增接口、质量门禁、从零启动 | ✅ |
| 门禁脚本 | `scripts/check-layering.mjs`、`check-secrets.mjs`、`quality-gate.mjs` | ✅ |
| 有效性验证（故意违规） | 见下文「四、规范有效性演练」 | ✅ |

### 4. 骨架

| 检查项 | 证据 | 状态 |
| --- | --- | --- |
| 分层清晰、无反向依赖 | `node scripts/check-layering.mjs` → 通过（13 包 / 25 条内部引用） | ✅ |
| 配置全外置、缺失即启动失败 | `internal/config` + 单测；实测 4 条失败路径 | ✅ |
| 免认证 `/health` 含版本/时间/依赖 | `GET /health`；单测 `TestHealthOK` | ✅ |
| 单行 JSON 日志 + 请求标识 | `internal/httpapi/middleware.go`；单测 `TestRequestIDAndAccessLog` | ✅ |
| 一条命令启动 | `node scripts/up.mjs` → 构建、启动、探活 | ✅ `deps={"mysql":"ok","qdrant":"ok"}` |
| 版本标签 | `v0.0.1-skeleton` | ⏳ 收尾时打 |
| 构建注入版本 | 镜像以 `VERSION=v0.0.1-skeleton` 构建，`/health` 返回同值 | ✅ |

### 5. 交付

| 检查项 | 证据 | 状态 |
| --- | --- | --- |
| 质量门禁全绿 | 见「三、实测记录」 | ✅ |
| 接口 e2e | `go run ./cmd/e2e` → **46 通过, 0 失败**，退出码 0 | ✅ |
| 浏览器 e2e | `node scripts/e2e-ui.mjs` → **13 通过, 0 失败**，退出码 0 | ✅ |
| 同伴评审 | `docs/lesson-07/peer-review.md` | ❌ 待第二位成员签署 |

## 二、架构与配置

依赖方向（门禁强制）：

```
cmd/server, cmd/e2e
        │
        ▼
internal/httpapi  ──(白名单文件)──▶ internal/store
        │
        ▼
internal/qa ──▶ store / retrieval / llm / qdrant
        │
        ▼
config / auth / db / deps / web（基础层，不依赖上层）
```

必填配置（无默认值，缺失即退出）：

| 键 | 示例 | 校验 |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | 非空 |
| `MYSQL_HOST` / `MYSQL_USER` / `MYSQL_PASSWORD` / `MYSQL_DATABASE` | `mysql` / `campusclaw` / 随机 / `campusclaw` | 非空 |
| `MYSQL_PORT` | `3306` | 1–65535 整数 |
| `QDRANT_URL` | `http://qdrant:6333` | http(s) 绝对地址 |
| `JWT_SECRET` | `openssl rand -hex 32` | ≥32 字符且非示例值 |

可选：`LLM_API_KEY`（空 = 抽取式降级；非空则必须同时给 `LLM_API_BASE`）、TTL、切片与阈值。

## 三、实测记录

### 1. 质量门禁

```bash
GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs
```

```
✓ 配置样例完整性
✓ 分层依赖 (64ms)
✓ 密钥与配置卫生 (76ms)
✓ gofmt (46ms)
✓ go build (951ms)
✓ go vet (261ms)
✓ go test (811ms)

质量门禁全部通过。
exit=0
```

### 2. 配置失败即退（spec R8）

以编译后的二进制逐项验证，均**单行 JSON、非零退出、只报键名**：

| 用例 | 输出摘要 | 退出码 |
| --- | --- | --- |
| 缺 `JWT_SECRET` | `配置校验失败: JWT_SECRET 未设置或为空` | 1 |
| 示例密钥 `replace-with-openssl-rand-hex-32` | `JWT_SECRET 仍是示例值，不能用于运行` | 1 |
| `MYSQL_PORT=99999` | `MYSQL_PORT 必须是 1–65535 的整数` | 1 |
| 配置齐全但依赖不可用 | `启动中止：依赖未就绪` + 逐项失败原因 | 1 |

单元测试覆盖：`go test ./internal/config`（9 个用例，含缺项、示例密钥、短密钥、非法端口、降级模式、`DSNFromEnv`）。

### 3. 健康检查与请求标识（spec R9 / R10）

`go test ./internal/httpapi` 覆盖：

- `TestHealthOK`：200；`version` 非空、`time` 可解析 RFC3339、`deps` 全 `ok`；响应不含 `password`/`secret`/`api_key`
- `TestHealthDegraded`：MySQL 失败 → 503，`deps.qdrant` 仍为 `ok`
- `TestRequestIDAndAccessLog`：`X-Request-ID: peer-123` 原样回写；访问日志为单行 JSON，含 `request_id`/`method`/`path`/`status`/`duration_ms`
- `TestInvalidRequestIDIsReplaced`：超长（200 字符）与含控制字符的 ID 被替换为 32 位十六进制

### 4. OpenSpec 严格校验

```bash
npx --yes @fission-ai/openspec@latest validate --all --strict
# ✓ change/2026-10-28-lesson-07-baseline
# ✓ spec/auth-upload
# ✓ spec/knowledge-retrieval
# Totals: 3 passed, 0 failed
```

### 5. 新会话可执行性抽查

另起一个**无上下文只读代理**（仅给仓库路径与问题，不给本会话历史），要求它回答「新增 `GET /api/materials/{id}` 按规范怎么做」。它自行找到并引用了：

- `AGENTS.md`（变更流程、分层、门禁）
- `.qoder/rules/01-product-design.md`、`02-backend.md`、`04-testing.md`、`08-review-and-done.md`
- `.qoder/skills/add-backend-endpoint/SKILL.md`、`run-quality-gate/SKILL.md`

回答与规范一致：先写四件套与 GIVEN/WHEN/THEN；handler 落 `httpapi` 并走 `requireAuth`；单表读用 store（白名单）；补 e2e 负向；提交前跑 `quality-gate.mjs`。说明规范对新会话是可发现、可执行的。

### 6. 容器与端到端实测

`node scripts/up.mjs`（首次构建前端 + Go 镜像，随后启动三容器）：

```
就绪：version=dev time=2026-10-28T02:16:40Z deps={"mysql":"ok","qdrant":"ok"}
```

以 `VERSION=v0.0.1-skeleton` 重新构建后，`/health` 返回 `version=v0.0.1-skeleton`。

**实时探活与请求标识**（`curl -H "X-Request-ID: peer-123"`）：

```
HTTP/1.1 200 OK
X-Request-Id: peer-123
{"deps":{"mysql":"ok","qdrant":"ok"},"llm":{"enabled":false},"status":"ok",
 "time":"2026-10-28T02:17:02Z","version":"dev"}
```

超长 ID（200 字符）被替换为 32 位十六进制；`docker compose logs backend` 中对应访问日志为：

```
{"level":"INFO","msg":"http_request","request_id":"peer-123","method":"GET",
 "path":"/health","status":200,"duration_ms":1,...}
```

**依赖故障**：停掉 Qdrant 后 `/health` 返回 **503**，`deps.mysql` 仍为 `ok`，`deps.qdrant` 为超时原因；
重启 Qdrant 后第一次探测即恢复 200。

**端到端**：

| 套件 | 结果 | 退出码 |
| --- | --- | --- |
| `go run ./cmd/e2e` | 46 通过, 0 失败 | 0 |
| `node scripts/e2e-ui.mjs` | 13 通过, 0 失败 | 0 |

Go e2e 较第一轮新增 5 项：`/health` 的 version、time、不泄漏密钥，以及 request ID 透传与非法值替换。

## 四、规范有效性演练

| 演练 | 操作 | 预期 | 实际 |
| --- | --- | --- | --- |
| 分层门禁 | 临时新增 `httpapi/_probe_layering.go`，import `internal/qdrant` | 失败并指名文件 | ✅ `httpapi 不得直接引用 internal/qdrant`，exit=1 |
| 密钥门禁 | 临时把 compose 的 `JWT_SECRET` 改为 `${JWT_SECRET:-dev-insecure-secret-change-me}` | 失败并指明回落默认值 | ✅ `JWT_SECRET 不得提供 :- 回落默认值`，exit=1 |
| 还原 | 删除探针文件、恢复 `${JWT_SECRET:?…}` | 两项复查通过 | ✅ exit=0 |

## 五、未完成与待办

1. **同伴评审**：必须由另一位成员按 `peer-review.md` 逐项检查并签署；当前表内为空，未代填。
2. **仓库改名**：`team_project` 不符合「小写短名」要求，建议改为 `campusclaw`；改名会影响课程提交地址，需确认后执行。
3. **分支保护**：`main` 需配置 PR + 至少 1 人评审 + 禁止直推；配置后日常改动走分支与 PR。
