# 后端开发规范

适用范围：`backend/` 下所有 Go 代码。

## 规则

1. **分层单向**：`cmd → httpapi → qa → store/retrieval/llm/qdrant`；禁止反向依赖。新包必须在 `scripts/check-layering.mjs` 登记层级。
2. **handler 不写业务**：httpapi 只做解析、鉴权、编排与响应；检索、生成、索引逻辑放 qa/store。直接使用 store 仅限白名单文件（单表读写与会话记录）。
3. **配置失败即退**：必填项缺失或非法时不得带默认值启动（见 AGENTS.md 第三节）。
4. **错误不吞**：错误必须 `%w` 包装并向上返回；只有明确的「可忽略」分支才允许丢弃，且写注释说明。
5. **context 贯穿**：所有外部调用（DB、HTTP、模型、向量库）必须接收并传递 `context.Context`。
6. **SQL 带租户**：业务查询必须带 `class_id`；列表必须有唯一 tiebreaker；多表写操作用事务。
7. **幂等**：种子、迁移、重索引可重复执行；写接口要么天然幂等，要么显式拒绝重复。
8. **结构化日志**：使用 `log/slog`，禁止 `log.Printf` 拼接敏感值。
9. **响应统一**：错误走 `writeErr`，成功走 `writeJSON`；不得手写 JSON 字符串。

## 检查方式

- `node scripts/check-layering.mjs`
- `gofmt -l .`、`go build ./...`、`go vet ./...`、`go test ./...`（含在 `quality-gate.mjs`）
- e2e 负向用例（401/403/404/400/503）

## 违规处理

- 门禁失败阻断合并；禁止 `--no-verify` 绕过。
- 已合入的分层违规：下一提交修复，并在 tasks 记录根因；连续两次同类违规需在评审会复盘。

## 有效性验证

新增一个「故意反向 import」的临时文件，确认 `check-layering.mjs` 失败后删除；记录在 `docs/lesson-07/verification.md`。
