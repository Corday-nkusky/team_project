## 实现

- [x] 恢复原集合及配套工具。verify: Go QA 用例核对搜索路径、向量命中和向量失败时词面降级。
  - 实测：`go test ./...` 通过。临时 Qdrant 只包含 `team_campusclaw_chunks`，模拟网关验收写入的向量在 Web 和 Worker 重启后可直接查询，SSE 返回向量引用。基础 E2E 81 通过、0 失败。
- [x] 验证模型验收和密钥修复。verify: `node scripts/quality-gate.mjs --with-spec` 退出 0。
  - 实测：隔离容器中的临时 Git 检出副本运行质量门禁全部通过，包含模型验收与密钥门禁回归和 OpenSpec 严格校验。真实 MySQL/Qdrant 搭配模拟网关执行 `node scripts/verify-llm.mjs` 全部通过。
- [x] 同步规约与归档。verify: `openspec archive` 成功，主规约严格校验通过。
  - 实测：归档退出 0，主规约 LG2 恢复固定集合并移除 LG4 的集合拆分要求。归档后 `openspec validate --all --strict` 13 通过、0 失败。
- [ ] 同伴评审。verify: 另一位成员在 docs/lesson-07/peer-review.md 留下评审记录。
