# Proposal: 补齐课程要求的「技能」与「MCP 工具接入」两项 AI 能力

## Why

课程首页明确要求小组项目「均须接入大语言模型、智能体、知识库、技能与 MCP」。本仓已具备大模型网关、智能体（流式问答）、知识库三项，但**技能路由与 MCP 工具层完全缺失**：

- 第 5 课要求教师维护至少一条「解题引导」类技能（只引导、不直接给答案），并有开关，关闭后同一问题行为可观测地变化；本仓 `ask` 链路写死 `qa.SystemPrompt()`，没有任何技能配置入口。
- 第 6 课要求自建 `mcp-server/`（JSON-RPC 2.0 三方法、令牌鉴权、≥2 工具）、宿主侧登记/连接/发现/断开、工具引用 `工具名@服务器名`、保存前校验、决策归模型执行归宿主、调用记录；本仓无 `mcp-server/`、无工具表、无调用记录。

这两项是结课硬要求的缺口，不是可选优化。不做，第 11 课报告「AI 写了什么」与五项能力接入一项就无法自证。

## What Changes

- 新增 `skills` 能力：教师按班维护解题引导技能（角色提示词 + 是否直达答案 + 工具引用），学生提问时由服务端按班加载启用的技能注入生成链路；关闭技能/切换模式后同一问题行为可观测变化。
- 新增 `mcp-tools` 能力：新增独立 `mcp-server/` 子工程（`initialize` / `tools/list` / `tools/call` + `/health`，令牌环境变量、至少两个工具）；宿主侧登记、连接（握手 + 发现两步）、断开、工具清单、调用记录全套接口；技能支持 `工具名@服务器名` 引用并在保存前校验；每次调用留只增不改的记录。
- 问答链路增量接入：检索/流式/降级主路径保持不变；有已连接技能时把工具清单交给模型，模型提出调用后由宿主校验参数并执行，结果作为工具消息回传，超限优雅降级。
- 保持班级隔离：技能、工具服务、调用记录全部按 `class_id` 约束；跨班一律 404。

## Capabilities

### New Capabilities

- `skills`：解题引导技能的配置、按班加载、开关行为变化。
- `mcp-tools`：MCP 工具服务、宿主登记连接、工具发现绑定、调用与记录。

### Modified Capabilities

- `knowledge-retrieval`：R5 增加「有技能时生成链路加载技能提示词与已发现工具」场景，检索与未命中判定不变。

## Impact

- 数据库：新增 `skills`、`mcp_servers`、`mcp_tool_calls` 三张表（幂等 `CREATE TABLE IF NOT EXISTS`，向前兼容）。
- 后端：新增 `internal/skill`（层 1）、扩展 `internal/qa` 的窄接口与提示词、扩展 `internal/store` 单表读写、`internal/httpapi` 新增 8 个路由；新增 `cmd/mcp-server` 独立入口。
- 部署：`deploy/docker-compose.yml` 增加 `mcp-server` 服务（同网、`MCP_AUTH_TOKEN` 环境变量）；`deploy/.env.example` 增加键名占位。
- 前端：`App.tsx` 增加教师侧「技能与工具」页；`api.ts` 增加类型与函数。
- 门禁：`scripts/check-layering.mjs` 登记 `internal/skill` 与 `cmd/mcp-server` 层级。
- 不改：认证授权、检索链路、异步索引、历史问答、密钥处理规则。
