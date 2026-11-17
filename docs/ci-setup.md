# CI 与主干保护设置

对应课程第 9/11 课门禁要求与「未经审核的 AI 原生代码不得合入 main」。

## 已有配置

- `.github/workflows/quality-gate.yml`：push / PR 到 `main` 时跑两个 job：
  - `check`：`make check`（= 配置样例 / 分层 / 密钥 / gofmt / build / vet / test）
  - `check-security`：`make check-security`（= 密钥扫描 / vet / 全量单测，含权限、越权、注入负向用例）
- 本地等价命令：`make check`、`make check-security`；Windows 未装 make 时用
  `node scripts/make-check.mjs` 与 `node scripts/make-check.mjs security`（调用同一套门禁脚本）。

## 需在 GitHub 仓库设置里完成的一次性配置（本仓文件无法代替）

1. **分支保护**：Settings → Branches → Add branch ruleset，目标 `main`：
   - Require a pull request before merging（建议 ≥1 approval）
   - Require status checks to pass：勾选 `check` 与 `check-security`
   - Do not allow bypassing the above settings
   - Require branches to be up to date before merging
2. **Actions 权限**：Settings → Actions → General → Workflow permissions，选择
   "Read repository contents and packages access"（本工作流只需读代码）。
3. 完成后在 README 或 `reports/gate-red-green.md` 记录开启日期与截图路径。

## 验证方法

- 本地：`make check` 与 `make check-security` 退出码均为 0（实测见 `openspec/changes/2026-11-17-add-skills-and-mcp-tools/tasks.md` 第 6 节）。
- CI：在 PR 页检查两个 check 均为 green；尝试在没有 approval 时合入 main，应被保护规则拒绝。
