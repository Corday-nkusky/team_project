# CI 与主干保护设置

对应课程第 9/11 课门禁要求与「未经审核的 AI 原生代码不得合入 main」。

## 已有配置

- `.github/workflows/quality-gate.yml`：push / PR 到 `main` 时跑三个 job：
  - `check`：`make check`（= 配置样例 / 分层 / 密钥 / gofmt / build / vet / test）
  - `check-security`：`make check-security`（= 密钥扫描 / vet / 全量单测 / SQL、XSS、CSRF 常驻反例 / govulncheck / npm 高危审计）
  - `image-version`：以 `${GITHUB_SHA}` 构建镜像，启动后断言 `/health.version` 等于该提交；运行镜像不包含 Node 或 Go 工具链
- 本地等价命令：`make check`、`make check-security`；Windows 未装 make 时用
  `node scripts/make-check.mjs` 与 `node scripts/make-check.mjs security`（调用同一套门禁脚本）。

## 需在 GitHub 仓库设置里完成的一次性配置（本仓文件无法代替）

1. **分支保护**：Settings → Branches → Add branch ruleset，目标 `main`：
   - Require a pull request before merging（建议 ≥1 approval）
   - Require status checks to pass：勾选 `check`、`check-security` 与 `image-version`
   - Do not allow bypassing the above settings
   - Require branches to be up to date before merging
2. **Actions 权限**：Settings → Actions → General → Workflow permissions，选择
   "Read repository contents and packages access"（本工作流只需读代码）。
3. 完成后在 README 或 `reports/gate-red-green.md` 记录开启日期与截图路径。

## 验证方法

- 本地：`make check` 与 `make check-security` 退出码均为 0（实测见 `openspec/changes/2026-11-17-add-skills-and-mcp-tools/tasks.md` 第 6 节）。
- CI：在 PR 页检查三个 job 均为 green；尝试在没有 approval 时合入 main，应被保护规则拒绝。
- 本地镜像版本复跑：`docker build -f deploy/backend.Dockerfile --build-arg VERSION=<提交SHA> -t campusclaw:<提交SHA> .`，以 `REQUIRE_DEPS=false` 启动无依赖 smoke 后核对 `/health.version`。此时 HTTP 503 与依赖不可用为预期，只证明版本注入，不冒充完整链路验收；验证失败时先执行 `docker logs campusclaw-version` 再清理容器。2026-11-21 本机以 `local-final-smoke-20261121` 实测一致，见 `reports/image-version-smoke.json`；GitHub workflow 需推送后由 Actions 实际执行，本地文件检查不冒充 CI 已通过。
