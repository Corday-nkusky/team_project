# 审核记录 02 — 技能与 MCP 工具接入（新增代码）

> 对应 `openspec/changes/2026-11-17-add-skills-and-mcp-tools`。四件事：手段 / 查了什么 / 判据 / 结果。
> 手段标注：本份含**脚本门禁 + AI 自审 + 1 条人工确认**（人工确认 ≤1，符合课程要求），且人工确认补了一条用例/规则。

## 审核对象

- `backend/internal/{skill,mcpclient}/*`（新增层 1 / 层 0 包）
- `backend/internal/httpapi/{skills,mcp,ask-tools}.go` 与 `ask.go` 的 meta 扩展
- `backend/internal/store/{skills,mcp}.go`、`backend/cmd/mcp-server/main.go`
- `frontend/src/pages/SkillsPage.tsx`、`frontend/public/{h5,swagger}.html`

## 审核项 1：接口入口（手段：AI 自审 + 单测）

- **查了什么**：技能与 MCP 全部 8 条新路由的角色与班级约束。
- **判据**：管理操作仅教师；学生配置被拒；跨班 id 404；令牌不回传。
- **结果**：单测 `TestUpsertSkillStudentForbidden`(403) / `TestDeleteSkillCrossClass404`(404) / `TestStudentCannotListMCPCalls`(403) / `TestMCPToolsCrossClass404`(404) / `TestListMCPServersNeverReturnsToken`（响应无 `SECRET_TOKEN` 与 `auth_token`，只有 `token_configured:true`）全部通过。
- **结论**：可用。

## 审核项 2：业务逻辑（手段：脚本门禁 + 规约对照）

- **查了什么**：OpenSpec delta 的 Scenario 与主规约一致性、strict 校验。
- **判据**：`validate --all --strict` 通过；每个 Requirement 有失败路径。
- **结果**：`Totals: 14 passed, 0 failed`。过程中修了 1 处：`rebuild-rag-pipeline` 的 R5 MODIFIED delta 缺 R5.10/R5.11，把它补齐后 strict 变绿（归档不会丢场景）。规约与实现一致。
- **结论**：可用。

## 审核项 3：数据库（手段：AI 自审 + 代码走查）

- **查了什么**：三张新表的 `class_id` 约束与调用记录的只增不改。
- **判据**：所有查询带 class_id；`mcp_tool_calls` 无 UPDATE/DELETE 路径；唯一性靠约束。
- **结果**：`skills` 有 `uk_skill_class_name(class_id,name)`，`mcp_servers` 有 `uk_mcp_class_name`，`mcp_tool_calls` 只有 INSERT 与按 class_id 的 SELECT；`GetMCPAuthToken` 是唯一读令牌路径且不进响应。未发现跨班查询。
- **结论**：可用。

## 审核项 4：公共函数（手段：AI 自审）

- **查了什么**：`ask.go` 的两处降级路径是否重复了 fallback 拼装逻辑。
- **判据**：同一段逻辑不应复制到多个文件。
- **结果**：发现两处降级（无网关 / 上游失败）各自拼 `fallback += tool notes + DecorateFallback`。提取为 helper 更干净，但两处仅 3 行且分属不同状态分支，判为可接受的局部重复；记入待改。
- **结论**：待修改（影响小，不阻断）。

## 审核项 5：错误处理（手段：AI 自审 + 单测）

- **查了什么**：技能加载失败、工具执行失败是否影响问答主路径。
- **判据**：技能/工具故障不得返回 5xx；按无技能/无工具继续并留 ERROR 日志。
- **结果**：`TestReaderFailureDegradesToBase` 通过；`prepareAsk` 对技能/服务器/令牌读取失败都 `slog.Error` 后继续；`recordToolCall` 失败只记日志不改回答。符合判据。
- **结论**：可用。

## 审核项 6：安全与隐私（手段：脚本门禁 + 单测）

- **查了什么**：MCP 令牌是否进代码/前端/镜像/日志；工具结果中的指令句式是否被丢弃。
- **判据**：令牌只从环境变量读，无默认值；响应无明文；工具结果含"忽略以上指令"整条丢弃。
- **结果**：`mcp-server` 无令牌即退出；`check-secrets.mjs` 通过；`TestClientAgainstStub` 错令牌报错；`TestExecuteFiltersInstruction` 通过；前端只有密码输入框，无令牌存储。
- **结论**：可用。

## 审核项 7：性能（手段：代码走查）

- **查了什么**：`connectedToolsByServer` / `prepareAsk` 是否每次提问多次扫库。
- **判据**：不应出现循环内逐条查库；工具调用有上限。
- **结果**：`prepareAsk` 一次 `ListMCPServers` 后在内存建索引，不在循环里查库；`WithinLimits` 限制 round ≤3、calls <4。判据满足。
- **结论**：可用。

## 人工确认项（唯一一条，已补规则）

- **查了什么**：无模型网关时"决策归模型"如何自证——规则要求模型决定调用，但零密钥环境没有模型。
- **手段**：**人工确认**（需求意图，机器判不了）。
- **判据**：降级路径仍须可演示、可留记录，且不能假装有模型决策。
- **结果**：决定按技能显式 `tool_refs` 执行一次，并在 `design.md` 与 `openapi.json` 的 `/api/ask` 描述里写明"无网关时按显式 tool_refs 降级"。
- **补的规则/用例**：在 spec `MC3.1` 与 `apitest/cases/api/ask/POST.py` 增加"无网关显式绑定 → meta.tool_calls 与调用记录一致"的端到端断言，使该人工判断固化为可回归用例。
- **结论**：可用（人工确认已补用例，不留口头默契）。

## 汇总

| 维度 | 结论 |
| --- | --- |
| 需求覆盖率 | 可用（8 条路由 + 端到端用例） |
| 与需求一致性 | 可用（strict 14/14） |
| 失败路径覆盖 | 可用（403/404/400/令牌/拒绝/超限/指令丢弃） |
| 查询性能风险 | 可用（无循环查库、调用有上限） |
| 重复代码比例 | 待修改（两处降级 3 行局部重复） |
| 漏洞风险 | 可用（令牌不回传、指令过滤、越权拒绝） |
| 高风险场景 | 并发/事务/权限分别见 `risk-concurrency.md` / `risk-transaction.md` / `risk-authz.md` |

**总体结论**：可用（1 项待修改：两处降级拼装的局部重复，不阻断结课）。
**审核人 / 手段**：AI 自审 + 脚本门禁 + 1 条人工确认（已补用例）。
