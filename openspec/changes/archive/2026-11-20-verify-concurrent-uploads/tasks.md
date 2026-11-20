## Tasks

- [x] 并发发送与持久化验证。verify: 并发专用入口退出 0；两次 202、同标识，材料/任务/切片/向量各一份。实测：`BASE=http://127.0.0.1:8081 USER=teacher1 node scripts/async-criteria.mjs --criterion concurrent` 退出 0，客户端重叠 5.04 ms，四种计数均为 1。
- [x] 归档实测与规范。verify: 质量门禁、OpenSpec 严格校验通过，风险报告可复跑。实测：`node scripts/quality-gate.mjs` 七项通过；`openspec validate 2026-11-20-verify-concurrent-uploads --strict` 通过。
