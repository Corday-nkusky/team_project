## Tasks

- [x] 验证任务写入回滚与同请求重试。verify: 真实 MySQL 运行 TestUploadRollbackAndRetry 两次均通过。实测：在 backend/ 执行 `go test -tags=mysqlintegration ./internal/store -run "^TestUploadRollbackAndRetry$" -count=2 -v`，2/2 PASS。临时容器内把材料写入移出事务后，相同用例报 materials=1 tasks=0，退出 1。
- [x] 接入 CI 并补齐复跑说明。verify: 项目质量门禁与 OpenSpec 严格校验通过。实测：`node scripts/quality-gate.mjs` 七项通过；`openspec validate 2026-11-19-test-upload-rollback --strict` 退出 0。
