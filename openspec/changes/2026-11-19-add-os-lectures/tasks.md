# Tasks

- [x] 1. 导出六份最终讲义及独立授权记录。verify: `node seed-data/operating-systems/seed.mjs --check`。实测（2026-11-19）：六份通过，UTF-8 正文与 LF 归一摘要一致；授权单独保存。
- [x] 2. 实现复用导入与负向检查。verify: `node --test seed-data/operating-systems/seed.test.mjs`。实测：Windows 宿主和 Linux 工具容器均 7/7 通过，覆盖首次导入、去重、学生/跨班拒绝、冲突拒绝、索引失败与数据校验。
- [x] 3. 质量门禁与规约校验。verify: `node scripts/quality-gate.mjs`；`npx --yes @fission-ai/openspec@latest validate 2026-11-19-add-os-lectures --strict`。实测：Node 22、Go 1.27 工具容器内配置/分层/密钥/gofmt/build/vet/test 全绿，严格校验 valid；前端 npm ci 与 npm run build 通过。
- [x] 4. 真实服务核验。verify: `node seed-data/operating-systems/seed.mjs`（默认 teacher1）。实测：已有 6 份 ready，新受理 0、跳过 6、拒绝 0、失败 0；材料总数仍为 6。
- [ ] 5. 另一位成员评审及归档。verify: 同伴完成 docs/lesson-07/peer-review.md 对应记录，执行 openspec archive 并同步主规约。待组员评审，不由作者代签。
