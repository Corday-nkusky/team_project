# 审核记录 01 — 第 8 课安全自查交叉复核

> 对应第 8 课要求的首份审核记录，也是最终报告「审核记录」一节的第一份。
> 四件事：用什么手段查 / 查了什么 / 判据 / 结果。手段标注：脚本门禁 / AI 自审 / 人工确认。
> 三分法：可用 / 待修改 / 风险。本份以**脚本门禁**为主。

## 审核对象

- 提交范围：`backend/internal/httpapi/{router,auth,middleware,materials,ask}.go`、`backend/internal/db/db.go`、`backend/internal/store/*`。
- 生成方式：AI 原生代码（Harness 生成，规约先行于 `openspec/`）。

## 审核项 1：接口入口参数校验与越权（手段：脚本门禁 + 单测）

- **查了什么**：`requireAuth` 对 401 的统一处理、上传接口的教师角色、跨班材料读取。
- **判据**：未登录一律 401；学生写操作 403；跨班读取 404 且不泄漏资源存在性；响应无多余字段。
- **结果**：e2e 实测 `账号不存在 401` / `口令错误 401` / `无 Authorization 401` / `篡改令牌 401`（`backend/cmd/e2e/main.go:179-190`）、`学生上传 403`（:219）、`A 班读 B 班材料 404`（:631-639）。全部符合判据。
- **结论**：可用。

## 审核项 2：密钥与配置卫生（手段：脚本门禁）

- **查了什么**：全部已跟踪文件的密钥明文形态、`.env.example` 占位、compose 的 `: -` 回落。
- **判据**：`node scripts/check-secrets.mjs` 退出码 0；注入假密钥时应退出码 1。
- **结果**：真实扫描 166 文件 0 命中（退出码 0）；先红后绿实验注入假 `sk-` 密钥 → 退出码 1，移除 → 0，见 `reports/gate-red-green.md`。
- **结论**：可用；门禁确实会拦，不是摆设。

## 审核项 3：数据库唯一性与幂等（手段：脚本门禁 + 代码走查）

- **查了什么**：`materials.content_hash`、`chunks.seq`、`index_tasks.idempotency_key` 的唯一约束；是否靠"先查后写"。
- **判据**：唯一性必须由数据库约束保证；重复提交不得产生重复记录。
- **结果**：`db.go` 三处 `UNIQUE KEY` 齐备；上传走 `ContentHash` 派生幂等键 + 唯一约束冲突复用任务；e2e `他班教师查询本班任务 404`：262 断言通过。未发现"先查后写"保证唯一性的写法。
- **结论**：可用。

## 审核项 4：分层依赖（手段：脚本门禁）

- **查了什么**：`node scripts/check-layering.mjs` 的反向依赖与 httpapi 直连限制。
- **判据**：只允许同层或向下依赖；httpapi 不直接 import llm/qdrant/retrieval。
- **结果**：`✓ 分层依赖检查通过`；无反向依赖。
- **结论**：可用。

## 审核项 5：错误处理（手段：代码走查 + 单测）

- **查了什么**：`writeErr` 统一错误体、`logServerError` 只记 request_id/operation/error、审计失败不回滚回答。
- **判据**：错误体统一为 `{"error":{"code","message"}}`；日志不含请求体/令牌/查询串。
- **结果**：单测 `TestUnknownAPIPathReturnsJSON404` 通过；`withLogging` 只记录 method/path/status/duration/request_id（R10）。未发现把内部路径或配置写进响应的分支。
- **结论**：可用。

## 汇总

| 维度 | 结论 |
| --- | --- |
| 需求覆盖率 | 可用（SEC-01～05 绿、SEC-06 红、SEC-07/08 黄，见 `security-fix-log.md`） |
| 与需求的一致性 | 可用（SEC-01～08 逐条对齐课程自查项，见 `security-precheck.md`） |
| 失败路径覆盖 | 可用（401/403/404/400/413/415/尾随 JSON 均有断言） |
| 查询性能风险 | 待修改（SEC-06 依赖审计已跑但高危未清零，见 `security-baseline.md`；压测未做 → 见 `bench.md` 待补） |
| 重复代码比例 | 未涉及（本轮审核对象是安全证据，不是新增业务代码） |
| 漏洞风险 | 可用（脚本门禁 + e2e 实测；SEC-06 红见 `security-baseline.md`） |
| 高风险场景 | 并发/事务/权限分别见 `risk-concurrency.md` / `risk-transaction.md` / `risk-authz.md` |

**总体结论**：可用（带一条待办：SEC-06 已逐条登记，待 Go ≥ 1.27.4 与 Vite 升级后转绿）。
**审核人 / 手段**：AI 自审 + 脚本门禁；本轮无人工确认项。
