# 可追溯矩阵（规约场景 → 用例）

> 课程第 10 课要求：每个规约场景至少一条用例，空着的场景优先补。
> 统计（2026-11-17）：9 份主规约共 **138** 个 Scenario；Go 测试函数 **115** 个、e2e 断言 **85** 个，另有 Node/浏览器脚本。
> 用法：改规约先改本表；改用例先查本表。未覆盖的标“—”并在文末列缺口。

## auth-upload（20 场景）

| 场景 | 用例 |
| --- | --- |
| R1.1–R1.7 登录 / 令牌 / 轮换 / 登出 | `backend/internal/httpapi/auth.go` 相关单测；`backend/cmd/e2e/main.go` :179-211（401/403/令牌轮换/登出失效） |
| R2.1–R2.3 上传与角色 | `materials_test.go`；e2e :219（学生上传 403） |
| R3.1–R3.8 材料与索引状态 | `internal/httpapi/materials_test.go`、`internal/qa/qa_test.go`；e2e 上传→索引 ready 链路 |
| R4.1–R4.2 输入边界 | e2e `checkHTTPContract`（500 :500）：413/415/尾随 JSON/未知路径 404 |

## knowledge-retrieval（24 场景）

| 场景 | 用例 |
| --- | --- |
| R5.1–R5.9 检索与依据门槛 | `internal/qa/qa_test.go`、`internal/retrieval/*_test.go`；e2e R5.2 未命中、R2.2 跨班检索 |
| R5.10–R5.11 技能叠加 | `internal/skill/skill_test.go`（`TestEnabledSkillComposesPrompt`、`TestReaderFailureDegradesToBase`）；apitest `cases/api/ask/POST.py` |
| R6.1–R6.6 流式与中断 | `backend/internal/httpapi/ask_test.go`（SSE 帧 / 中断 / 降级 / done id） |
| R7.1–R7.7 引用回看 | `ask_test.go` `citationFilter`；`scripts/verify-history-ui.mjs`（U3/U6 全文与高亮）；e2e 引用逐条校验 |

## runtime-baseline（25 场景）

| 场景 | 用例 |
| --- | --- |
| R8.1–R8.6 配置外置 | `internal/config/config_test.go`（缺失 / 示例值 / 短密钥 / 非法端口） |
| R9.1–R9.5 健康与依赖 | `router_test.go`（`TestHealthOK`、`TestHealthReportsLLMModels`、降级 503） |
| R10.1–R10.6 日志与 request id | `middleware.go` + e2e R10 请求标识透传用例 |
| R11.1–R11.5 错误体与 HTTP 契约 | `response_test.go`、`router_test.go`；e2e `checkHTTPContract` |
| R12.1–R12.3 可复现启动 | `scripts/up.mjs` + `reports/async-criteria.md` 真机启动记录 |

## async-indexing（26 场景）

| 场景 | 用例 |
| --- | --- |
| A1.2、A1.4 任务写入回滚与原请求重试 | [TestUploadRollbackAndRetry](../backend/internal/store/upload_rollback_test.go)；[MySQL 复跑说明](../reports/risk-transaction.md) |
| A1–A3 幂等与事务性发件箱 | `internal/store/tasks_test.go`；`reports/risk-concurrency.md`、`reports/risk-transaction.md`（真机输出） |
| A4–A6 worker 抢占 / 租约 / 重试 | `internal/indexer/*_test.go`；`reports/async-criteria.md` 判据二（停 worker / 恢复） |
| A7–A9 状态机与终态 | e2e 轮询任务终态；`scripts/verify-h5.mjs` 第 3 步（pending→running→succeeded） |

## ask-history（10 场景）

| 场景 | 用例 |
| --- | --- |
| H1.1–H1.4 历史只读查询 | `internal/httpapi/conversations_test.go`（隔离 / 排序 / 上限 / 空 / 非法 limit / 查询失败不伪装空） |
| H2.1–H2.3 历史面板 | `scripts/verify-history-ui.mjs`（U1/U2/U3） |
| H3.1–H3.3 done 回执 | `ask_test.go`（`TestAskDoneCarriesConversationID` 等） |

## material-reading（10 场景）

| 场景 | 用例 |
| --- | --- |
| MR1.1–MR1.4 材料详情 | `materials_test.go`；e2e :309（无令牌详情 401） |
| MR2.1–MR2.6 阅读面板 | `scripts/verify-history-ui.mjs`（U3 全文 2400 字 / 高亮 342 字、U4 Esc、U5 材料页共用） |

## learning-workspace（8 场景）

| 场景 | 用例 |
| --- | --- |
| LW1.1–LW4.2 学习工作台 | 本轮未改；见 `openspec/specs/learning-workspace/spec.md` 与历史 change `2026-10-31-refine-learning-workspace/tasks.md` 的实测记录 |

## skills（5 场景，本次新增）

| 场景 | 用例 |
| --- | --- |
| SK1.1 教师新建列出 | `apitest/cases/api/skills/{GET,POST}.py`；`internal/httpapi/skills_test.go` |
| SK1.2 学生与跨班 | `TestUpsertSkillStudentForbidden`(403)、`TestDeleteSkillCrossClass404`(404) |
| SK2.1 开关行为差异 | `internal/skill/skill_test.go`（`TestDecorateFallbackDiffersBySwitch`）；apitest 端到端 |
| SK2.2 无技能逐字不变 | `TestNoSkillReturnsBasePromptVerbatim` |
| SK2.3 配置故障降级 | `TestReaderFailureDegradesToBase` |

## mcp-tools（11 场景，本次新增）

| 场景 | 用例 |
| --- | --- |
| MC1.1–MC1.2 三方法与令牌 | 真机 curl 5.1 节（health / 无令牌 -32001 / initialize / tools/list / 两个 call / method_not_found）；`internal/mcpclient/host_test.go` |
| MC2.1–MC2.2 登记连接 | `apitest/cases/api/mcp/servers/POST.py`（登记/连接/重复不翻倍/断开）；单测令牌不回传、跨班 404 |
| MC3.1–MC3.3 引用校验与编排 | `TestCheckToolRef`、`TestValidateRefs`、`TestValidateArguments`、`TestExecuteOK`、`TestExecuteFiltersInstruction`、`TestWithinLimits`；apitest 端到端 |
| MC4.1–MC4.2 调用记录 | `TestListMCPCallsInvalidLimit`、`TestMCPCallsHaveFiveFields`、`TestStudentCannotListMCPCalls` |
| MC5.1–MC5.2 Swagger/apitest | `/swagger.html` + `/openapi.json`（16 路径）；`python -m py_compile` 全通过 |

## 缺口（诚实）

- LW1–LW4：本轮未重跑学习工作台用例（无改动），沿用历史 change 的实测记录。
- 浏览器端 XSS 判定（看页面源码）与 CSRF SameSite 确认：未做真机截图，见 `reports/security-fix-log.md` SEC-08 黄判。
- apitest 端到端（连接→保存技能→提问→记录）已真机跑通 42/42，H5 四步链路全绿；原始输出见 `reports/apitest-run.log`、`reports/verify-h5.log`。手机浏览器点按未做，用 Node `fetch` 复现同一请求序列。
- 依赖审计（SEC-06）：已跑，高危未清零（Go 12 条可达 + npm 2 包含 1 high），逐条登记见 `reports/security-baseline.md`；升级 Go / Vite 后的回归未做。

## 2026-11-19 操作系统讲义
- OS1：seed-data/operating-systems/seed.mjs loadLectures；seed.test.mjs 的数据校验与缺文件/摘要负向断言。
- OS2：复用 biology/seed.mjs 的上传、去重与任务轮询；seed.test.mjs 覆盖导入、重复、身份、正文冲突、任务失败。
- 同伴评审待另一位成员完成，自审证据见 reports/os-lectures-review.md。
