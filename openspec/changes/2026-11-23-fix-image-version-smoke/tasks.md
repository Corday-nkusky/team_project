# Tasks: 修复镜像版本烟雾检查

## 1. 修复与验证

- [x] 1.1 修正 smoke 容器依赖模式并增加失败日志。verify: CI 同参数启动后 `/health` 返回 503、注入版本和不可用依赖状态；故意使用错误预期版本时断言非零且日志可见。
  - 实测：2026-11-23 `docker build -f deploy/backend.Dockerfile --build-arg VERSION=image-version-fix-20261123 -t campusclaw:image-version-fix .` 退出 0；以 CI 相同环境启动后 `/health` 为 503，`version=image-version-fix-20261123`，MySQL/Qdrant 均如实报告不可用。故意将期望值设为 `intentionally-wrong-version` 时断言非零并输出实际/期望值，`docker logs` 可见跳过依赖预检与 `server_starting`。烟雾容器已清理。
- [x] 1.2 同步 CI 文档并验证规约与质量门禁。verify: `npx @fission-ai/openspec@latest validate 2026-11-23-fix-image-version-smoke --strict` 与 `node scripts/quality-gate.mjs` 均退出 0。
  - 实测：OpenSpec 单项严格校验输出 `Change '2026-11-23-fix-image-version-smoke' is valid`；追加 `npx @fission-ai/openspec@latest validate --all --strict` 为 13 通过、0 失败。默认门禁首次因 Go 不在 PATH 停止；以 `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs` 重跑，配置、分层、密钥、gofmt、build、vet、test 全部通过，退出 0。
- [ ] 1.3 同伴评审。verify: 另一位成员批准对应 PR，并记录遗留项。
  - 实测：待 PR 评审。
