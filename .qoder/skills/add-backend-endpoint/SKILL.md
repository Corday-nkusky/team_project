---
name: add-backend-endpoint
description: 按 CampusClaw 规范新增或修改后端 HTTP 接口：从 OpenSpec 场景出发，落成分层代码、鉴权与审计、单元与 e2e 测试，最后跑通质量门禁。适用于在 backend/internal/httpapi 增加路由、在 internal/qa 或 internal/store 增加逻辑、或调整已有接口行为。
---

# 新增后端接口

## 开始前必读

- `AGENTS.md`（分层、配置、日志、接口约定）
- `.qoder/rules/02-backend.md`、`03-security.md`、`04-testing.md`
- 目标能力的 `openspec/specs/<capability>/spec.md`

## 步骤

### 1. 规格先行

在 `openspec/changes/YYYY-MM-DD-<verb>-<noun>/` 写清：

- `proposal.md`：价值三问、非目标、能力↔场景对照
- `specs/<capability>/spec.md`：Requirement + 场景，**GIVEN / WHEN / THEN** 齐全，至少一条失败路径
- `tasks.md`：每条任务带可判定的 `verify:`

没有场景编号（如 R5.2）就不写代码。

### 2. 决定代码落点

| 职责 | 位置 |
| --- | --- |
| 路由、参数解析、鉴权、响应 | `internal/httpapi/*.go` |
| 检索 / 生成 / 索引编排 | `internal/qa/qa.go` |
| 单表读写、事务、审计 | `internal/store/store.go` |
| 向量库、模型网关 | `internal/qdrant`、`internal/llm` |
| 配置 | `internal/config/config.go`（必填项加进 `.env.example`） |

禁止：handler 直接 import `llm`/`qdrant`/`retrieval`；新增内部包必须先在 `scripts/check-layering.mjs` 登记层级。

### 3. 实现

- 路由注册到 `httpapi/router.go`：公开 / `requireAuth` / `requireRole("teacher", …)`
- 错误统一 `writeErr(w, status, code, message)`；成功 `writeJSON`
- 业务查询必须带 `class_id`；列表排序带唯一 tiebreaker
- 属登录 / 上传 / 问答的动作必须写审计（`store` 内的 audit helper）
- 日志用 `slog`，不打印请求体、令牌、口令
- 流式接口必须透传 `http.Flusher`（`statusRecorder` 已实现，勿再包一层不转发 Flush 的 writer）

### 4. 测试

- 纯逻辑 / 边界：`internal/...` 下 Go 单测
- 接口行为：在 `backend/cmd/e2e/main.go` 增加断言，标题写 Requirement 编号
- 负向至少覆盖：未认证 401、越权 403/404、非法参数 400、依赖失败 503（按需）
- 测试数据用随机独有词，禁止依赖执行顺序

### 5. 文档

- `README.md`：接口表或行为变化
- `AGENTS.md`：若引入新约束
- `deploy/.env.example`：若新增配置项
- change 的 `tasks.md`：勾选并写「实测：命令 → 结果」

### 6. 门禁

```bash
node scripts/quality-gate.mjs
# Go 不在 PATH 时：
GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs
```

涉及接口时再跑 e2e（需服务与依赖已启动）：

```bash
GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-e2e
```

## 完成检查

- [ ] 每个新 Scenario 至少一条断言，且输出里带编号
- [ ] 跨班 / 角色负向用例存在
- [ ] 无敏感值进入日志或响应
- [ ] `quality-gate.mjs` 全绿
- [ ] tasks 已写实测并勾选
- [ ] 已请另一位成员评审

## 常见错误

- 在 handler 里直接调 Qdrant / LLM → 分层门禁会拦
- 忘记 `class_id` 过滤 → 先补 e2e 跨班用例再改代码
- SSE 包了不转发 `Flush` 的中间件 → 退化为假流式
- 新配置项写了默认值 → 违反 R8，门禁与单测都会拦
