# 审核记录 03 — 门禁、CI 与交付链路

> 四件事：手段 / 查了什么 / 判据 / 结果。本份以**脚本门禁 + 1 条人工确认**为主。
> 对应最终报告「门禁与演示」一节（20 分）。

## 审核对象

- `Makefile`、`scripts/make-check.mjs`、`.github/workflows/quality-gate.yml`
- `scripts/quality-gate.mjs`、`scripts/check-layering.mjs`、`scripts/check-secrets.mjs`
- `apitest/*`、`frontend/public/{openapi.json,swagger.html}`、`reports/gate-red-green.md`

## 审核项 1：门禁真的会拦（手段：脚本门禁 + 先红后绿）

- **查了什么**：`check-secrets.mjs` 对密钥明文的拦截能力。
- **判据**：正常状态退出码 0；注入假密钥退出码 1；移除后回到 0。
- **结果**：三步实测：绿(0) → 红(1) → 绿(0)，见 `reports/gate-red-green.md`。扫描文件数始终 166，说明变的只是文件内容。
- **结论**：可用。

## 审核项 2：门禁覆盖面（手段：脚本门禁）

- **查了什么**：`make check` 与 `make check-security` 的实际检查项。
- **判据**：配置样例 / 分层 / 密钥 / gofmt / build / vet / test 全跑；两条命令退出码 0。
- **结果**：`node scripts/make-check.mjs` 七项全绿退出码 0；`node scripts/make-check.mjs security` 全绿退出码 0（本机 Git Bash 未装 make，用 Node 等价入口；命令与 Makefile 调用同一套门禁脚本）。
- **结论**：可用。

## 审核项 3：CI 与本地同命令（手段：人工确认 + 配置走查）

- **查了什么**：CI 是否跑与本地相同的门禁；main 保护能否要求评审。
- **判据**：CI 两个 job（check / check-security）分别跑 `make check` / `make check-security`；main 保护需 review + status check。
- **结果**：`.github/workflows/quality-gate.yml` 两个 job 的命令与 Makefile 逐字一致；CI 先 `npm ci` + `npm run build` 再跑门禁。
- **人工确认项（唯一一条）**：GitHub 仓库的分支保护设置无法通过仓库文件完成，必须在 Settings → Branches 手动开启；本轮**未实机开启**，已在 `docs/ci-setup.md` 写清步骤与验证方法。这是机器判不了、需人在 GitHub UI 操作的项。
- **补的规则/用例**：在 `docs/ci-setup.md` 增加"完成后在 README 或 reports 记录开启日期与截图路径"的判定，使该人工动作有归档要求。
- **结论**：待修改（CI 文件正确；main 保护待人在 GitHub 设置，属 `[队]`/教师侧动作）。

## 审核项 4：OpenAPI 与实现一致（手段：脚本门禁 + 走查）

- **查了什么**：`/openapi.json` 是否覆盖新增 8 路由、错误码与是否需登录。
- **判据**：16 条路径；每个新接口有错误码与安全标记；JSON 合法。
- **结果**：`python json.load` 通过，16 条路径；`/api/skills`、`/api/mcp/*` 均标注 400/403/404 与 bearerAuth；`swagger.html` 从 localStorage 读 `cc_access` 注入 Authorization，可试调。
- **结论**：可用。

## 审核项 5：接口测试可回归（手段：ai 自审 + 语法检查）

- **查了什么**：`apitest/` 的按 URL 组织、一键运行、HTML 报告、端到端断言。
- **判据**：一个接口一个脚本；`run_all.py` 一键跑并生成报告；端到端核对技能保存内容与调用记录一致。
- **结果**：目录 `cases/api/{login,skills,mcp/servers,mcp/calls,ask}` 符合约定；`python -m py_compile` 全部通过；`run_all.py` 汇总 `check()` 并写 `apitest/report.html`（已 gitignore）。
- **结论**：可用（真机运行见未验证清单）。

## 汇总

| 维度 | 结论 |
| --- | --- |
| 需求覆盖率 | 可用（门禁覆盖 R8/R9/R10/R11/SK/MC） |
| 与需求的一致性 | 可用（CI 与本地 `make check` 同命令；OpenAPI 16 路径与路由一致） |
| 失败路径覆盖 | 可用（先红后绿 + 403/404/400 门禁） |
| 查询性能风险 | 未涉及（本轮审核对象为门禁与交付链路，不含查询路径） |
| 重复代码比例 | 未涉及 |
| 漏洞风险 | 可用（密钥扫描 0 命中、注入假值即红） |
| 高风险场景 | 见 `risk-concurrency.md` / `risk-transaction.md` / `risk-authz.md`；门禁层不重复评三查 |

**总体结论**：可用（1 项待修改：main 分支保护需人在 GitHub 设置）。
**审核人 / 手段**：AI 自审 + 脚本门禁 + 1 条人工确认（已补归档要求）。
