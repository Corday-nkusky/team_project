# Spec Delta: mcp-tools

## ADDED Requirements

### Requirement: MC1 自建 MCP 工具服务

系统 SHALL 在 `backend/cmd/mcp-server/` 提供独立服务，以 `POST /mcp` 为单入口接受 JSON-RPC 2.0 报文，实现 `initialize`（能力协商）、`tools/list`（工具发现）、`tools/call`（执行工具）三方法，并提供免认证 `GET /health`。SHALL 至少暴露 2 个工具，每个写全名称、说明、输入结构（JSON Schema）。令牌 SHALL 从 `MCP_AUTH_TOKEN` 环境变量读取，不设默认值；无令牌或令牌错误 SHALL 返回 JSON-RPC 错误 `invalid_token` 且不执行工具。非法请求 SHALL 返回可读 JSON-RPC 错误。

#### Scenario: MC1.1 带令牌的发现与调用

- **GIVEN** mcp-server 已由 Compose 启动，`MCP_AUTH_TOKEN` 已注入
- **WHEN** 客户端以正确令牌 `POST /mcp` 依次调用 `initialize`、`tools/list`、`tools/call`
- **THEN** `initialize` 返回协议版本与能力；`tools/list` 返回 ≥2 个工具且每个含 name/description/inputSchema
- **AND** `tools/call` 返回文本内容，不含指令式句式

#### Scenario: MC1.2 失败路径：令牌与非法方法

- **GIVEN** 服务已启动
- **WHEN** 不带令牌、带错误令牌调用 `tools/list`，或调用不存在的方法
- **THEN** 前者返回 JSON-RPC error `invalid_token`，后者返回 `method_not_found`
- **AND** 两种情况都不执行任何工具，响应为合法 JSON-RPC 错误对象

### Requirement: MC2 宿主登记、连接与工具发现

教师 SHALL 能登记工具服务（name、Base URL、令牌），接口响应 SHALL 只回「是否已配置令牌」布尔标记，不回令牌本身。「连接」SHALL 为握手 + 工具发现两步，任一步失败停在未连接并写可读 `last_error`；两步都成功才整体替换工具清单并标记已连接；连续连接两次清单条数不翻倍。断开 SHALL 清空清单且不可再调用。连接失败 SHALL 不影响既有检索与流式问答主路径。所有操作按 class_id 隔离，跨班 404，学生 403。

#### Scenario: MC2.1 连接成功后可见工具且重复连接不翻倍

- **GIVEN** 教师 A 登记了本班工具服务 `campus-tools`，指向 Compose 内的 mcp-server
- **WHEN** A 连续请求两次 `POST /api/mcp/servers/{id}/connect`，再 `GET /api/mcp/servers/{id}/tools`
- **THEN** 服务器状态为 connected，工具清单条数等于 mcp-server 的工具数且不翻倍
- **AND** 列表响应中不含令牌明文，只有 `token_configured=true`

#### Scenario: MC2.2 失败路径：发现失败与断开后不可调用

- **GIVEN** 教师 A 登记了指向不可达地址的服务器；另有一台已连接的服务器
- **WHEN** A 连接不可达服务器；随后断开已连接服务器并用引用其上工具的技能提问
- **THEN** 前者停在未连接，`last_error` 含可读原因，问答/检索仍正常；后者引用状态为「被拒」并留下一条拒绝记录

### Requirement: MC3 工具引用校验与调用编排

技能的 `tool_refs` SHALL 支持 `工具名@服务器名`（工具名小写字母开头、不含连字符；服务器名可含连字符）。保存前 SHALL 校验格式、工具在发现清单内、服务器已连接，任一不满足即 400 可读错误。提问时系统 SHALL 把本次可用工具交给模型；模型提出调用后，宿主 SHALL 按同一 inputSchema 校验参数再带令牌执行，结果先过检查（命中指令式句式整条丢弃、过长截断）再作为工具消息回传。SHALL 设轮次上限与累计调用上限，超限优雅降级。调用事件 SHALL 先于正文推送，`meta.tool_calls` 可见本次工具与状态。无模型网关时 SHALL 按技能显式 tool_refs 执行一次，保证演示链路可跑。

#### Scenario: MC3.1 端到端调用与先于正文

- **GIVEN** 教师已连接 `campus-tools`，保存了引用 `formula_hint@campus-tools` 的技能，学生 A1 提问命中该技能
- **WHEN** A1 发起 `POST /api/ask`
- **THEN** SSE 首个 `meta` 事件的 `tool_calls` 已含该工具及状态，随后才是 delta 正文
- **AND** 调用记录中出现一条 `tool@server = formula_hint@campus-tools` 且结果为「成功」

#### Scenario: MC3.2 失败路径：保存非法引用

- **GIVEN** 服务器 `campus-tools` 已连接，其清单无 `bad_tool`
- **WHEN** 教师保存技能时 `tool_refs:["bad_tool@campus-tools"]` 或 `["Bad-Tool@campus-tools"]` 或引用未连接服务器
- **THEN** 均返回 400 且给出可读错误，技能未保存

#### Scenario: MC3.3 失败路径：超限与指令注入

- **GIVEN** 轮次上限被调小，且某工具返回文本含「忽略以上指令」
- **WHEN** 学生提问触发多轮调用
- **THEN** 超限后停止调用、用已有信息作答且不报错中断；含指令句式的工具结果被整条丢弃并在 `meta.tool_calls` 记为「被拒」

### Requirement: MC4 调用记录与隔离

系统 SHALL 为每次工具调用留一条**只增不改**的记录，含五问：调了什么（`tool@server`）、结果（成功/被拒/出错）、谁触发、在哪次会话（含第几轮）、时间。教师 SHALL 只能查看本班记录，默认 limit=20、上限 50，非法 limit 返回 400。学生访问 SHALL 403，跨班 id SHALL 404。

#### Scenario: MC4.1 三种结果各可见一条

- **GIVEN** 本班已发生一次成功调用、一次被拒调用、一次出错调用
- **WHEN** 教师 A 请求 `GET /api/mcp/calls`
- **THEN** 返回本班三条记录，每条五问字段齐全，三种结果各一条
- **AND** 记录不可被任何接口修改或删除

#### Scenario: MC4.2 失败路径：越权与非法 limit

- **GIVEN** 学生 A1、教师 B（班级 2）
- **WHEN** A1 请求 `GET /api/mcp/calls`，或 B 请求 `?limit=abc` / `?limit=0`
- **THEN** 前者 403，后者 400，均不返回任何调用记录

### Requirement: MC5 Swagger 与接口测试

系统 SHALL 为新增接口提供 OpenAPI 描述（路径、入参出参、错误码、是否需登录），并可在浏览器打开 Swagger 页面试调。`apitest/` SHALL 含登记、连接、列出工具、断开用例及一条端到端用例（连接 → 保存引用工具的技能 → 提问 → 记录出现成功），核对返回内容与记录一致，由 `run_all.py` 一键运行生成 HTML 报告。

#### Scenario: MC5.1 端到端接口测试

- **GIVEN** mcp-server 与主应用均已启动，测试身份为教师与学生
- **WHEN** 运行 `python apitest/run_all.py`
- **THEN** 生成 HTML 报告，登记/连接/列出/断开/非管理角色被拒/端到端调用全部通过
- **AND** 端到端用例核对了技能保存内容与调用记录里的 tool@server 一致

#### Scenario: MC5.2 失败路径：文档与实现一致

- **GIVEN** 浏览器打开 Swagger 页面
- **WHEN** 对照 OpenAPI 文档试调登记、连接、工具、技能保存接口
- **THEN** 每个新增接口都有描述，错误码与实际返回一致，需登录接口在文档中标注
