## 实现

- [x] 暴露语义检索状态和引用来源并更新验收。verify: Go HTTP/QA 测试与 verify-llm 脚本测试均通过。
  - 实测：`go test ./...` 通过；`node --test scripts/verify-llm.test.mjs scripts/check-secrets.test.mjs` 共 5 项通过。真实 MySQL/Qdrant 与模拟网关环境中的 `node scripts/verify-llm.mjs` 全部通过。
- [x] 按嵌入空间隔离集合并更新迁移文档与验证工具。verify: Go QA/Qdrant 测试通过，同维度切换前后状态符合规约。
  - 实测：`go test ./...` 通过；临时 MySQL 8.4 和 Qdrant 1.12.4 环境切换同维嵌入模型后，原材料返回 `needs_reindex` 与词面引用，重索引完成后返回 `available` 与向量引用。 `go run ./cmd/e2e` 对应的独立 E2E 二进制在零密钥模式执行，81 通过、0 失败。
- [x] 补密钥门禁并脱敏网关错误。verify: Go LLM 测试与 Node 密钥门禁测试通过。
  - 实测：`go test ./...` 通过；`node --test scripts/check-secrets.test.mjs scripts/verify-llm.test.mjs` 通过，故意放入合成密钥和非占位 key 均使扫描器退出 1，输出未回显值。
- [x] 运行门禁与严格规格校验。verify: `node scripts/quality-gate.mjs --with-spec` 退出 0。
  - 实测：只读工作区的临时 Git 检出副本通过配置、分层、密钥、回归、gofmt、go build、go vet、go test 与 OpenSpec 严格校验。
- [x] 同步主规约与归档。verify: `openspec archive 2026-11-21-harden-model-gateway --yes` 退出 0。
  - 实测：归档退出 0，主规约同步 5 条需求，变更已移入 archive。补齐主规约 Purpose 和归档 proposal 的标准标题后，`openspec validate --all --strict` 为 13 通过、0 失败。
- [ ] 同伴评审。verify: 另一位成员在 `docs/lesson-07/peer-review.md` 记录评审。
