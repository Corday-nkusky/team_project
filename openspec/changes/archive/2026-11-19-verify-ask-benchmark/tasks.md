## Tasks

- [x] 核验事件并接入压测统计。verify: Node 测试验证正常回答、拒答和截断的计数、时延与退出码。实测：`node --test scripts/bench-ask-stream.test.mjs` 7/7 通过；本地 8081 单请求得到 no_evidence=1、errors=0。
- [x] 更新 CI 与复跑说明。verify: 项目质量门禁及 OpenSpec 严格校验通过。实测：`node scripts/quality-gate.mjs` 七项通过，`openspec validate 2026-11-19-verify-ask-benchmark --strict` 通过。
