## Tasks

- [x] 并发发送与持久化验证。verify: 并发专用入口退出 0；两次 202、同标识，材料/任务/切片/向量各一份；服务端两条访问日志的耗时区间重叠。实测：`BASE=http://127.0.0.1:8080 USER=teacher1 node scripts/async-criteria.mjs --criterion concurrent` 退出 0，客户端窗口重叠 12.91 ms，服务端日志反推起始相差 0.2 ms、在途重叠约 7.8 ms，四种计数均为 1（材料 389）。
- [x] 归档实测与规范。verify: 质量门禁、OpenSpec 严格校验通过，风险报告可复跑。实测：`node scripts/quality-gate.mjs` 七项通过；`openspec validate 2026-11-20-verify-concurrent-uploads --strict` 通过。
- [x] 未知参数不空跑。verify: `--criterion` 传未知值时退出码 2 且不覆盖已归档报告。实测：`node scripts/async-criteria.mjs --criterion concurent` 输出「只接受 all / 1 / 2 / 3 / concurrent」并以退出码 2 终止。
