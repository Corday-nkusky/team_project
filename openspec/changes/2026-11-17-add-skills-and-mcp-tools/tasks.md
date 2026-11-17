# Tasks: 补齐技能与 MCP 工具接入

> 实测在完成后回填（命令 + 结果）；真机未跑的边界在第 6 节如实标注。

## 1. 规约先行

- [x] 1.1 新建 change 四件套（proposal / design / 2 份 spec delta / tasks），每个 Requirement 含至少一个失败路径 Scenario
  - verify: `npx @fission-ai/openspec@latest validate --all --strict` 通过
  - 实测：`Totals: 14 passed, 0 failed`（含新 change 与 skills/mcp-tools 两份主规约）
- [x] 1.2 主规约同步：新增 `skills`、`mcp-tools`，更新 `knowledge-retrieval` 的 R5.10/R5.11 与对照表
  - verify: 主规约与 delta 逐字一致，strict 通过
  - 实测：主规约 R5 追加 R5.10/R5.11；`rebuild-rag-pipeline` 的 R5 delta 同步补齐两条，归档不会丢场景

## 2. 数据库与分层登记

- [x] 2.1 新增 `skills`、`mcp_servers`、`mcp_tool_calls` 三表（`IF NOT EXISTS`、class_id 约束、唯一键、审计字段）
  - verify: `go build ./...` 通过；真机重启后三表存在（真机见第 6 节）
  - 实测：DDL 写入 `internal/db/db.go` schema，全部 `CREATE TABLE IF NOT EXISTS`，启动幂等
- [x] 2.2 `scripts/check-layering.mjs` 登记 `internal/skill`(层1)、`internal/mcpclient`(层0)、`cmd/mcp-server`(层3)，httpapi store 白名单扩为含新单表读写文件
  - verify: `node scripts/check-layering.mjs` 通过
  - 实测：`✓ 分层依赖检查通过（18 个包，52 条内部引用）`

## 3. MCP 工具服务（mcp-server）

- [x] 3.1 `backend/cmd/mcp-server/main.go`：JSON-RPC 2.0 `POST /mcp` 三方法 + `/health`，标准库实现
  - verify: 带令牌 `tools/list` 返回 ≥2 工具且每项含 name/description/inputSchema
  - 实测：见第 5.1 节真机 HTTP，返回 `formula_hint`、`unit_convert` 两个工具，三项齐全
- [x] 3.2 令牌从 `MCP_AUTH_TOKEN` 读，无默认、不进代码/前端/日志；无/错令牌返回 `invalid_token`
  - verify: 无令牌与错令牌请求均返回 JSON-RPC error 且不执行工具
  - 实测：第 5.1 节无令牌请求返回 `{"error":{"code":-32001,"message":"invalid_token"}}`；`check-secrets.mjs` 通过
- [x] 3.3 两个工具实现：返回纯文本、不含指令式句式
  - verify: `tools/call` 两种工具各返回文本，断言无「忽略」「system」等指令词
  - 实测：第 5.1 节 `formula_hint` 返回分步线索、`unit_convert` 返回 `2000 m`；单测 `TestExecuteFiltersInstruction` 覆盖指令丢弃
- [x] 3.4 Compose 增加 `mcp-server` 服务（同网、令牌环境变量），Dockerfile 多产出一个二进制
  - verify: `docker compose up -d --build` 后健康（真机见第 6 节）
  - 实测：`docker compose config --quiet` 通过（配置合法）；Docker daemon 本机未运行，`up` 未执行

## 4. 技能（skills）

- [x] 4.1 `internal/skill`：按班加载启用技能、叠加提示词；无技能时提示词逐字不变
  - verify: 单测覆盖启用/关闭/无技能三态
  - 实测：`go test ./internal/skill` 通过 4 用例；`TestNoSkillReturnsBasePromptVerbatim` 锁定逐字不变
- [x] 4.2 `internal/store`：技能 CRUD（class_id 约束、version 递增、唯一 name）
  - verify: 单测/真机覆盖本班读写、跨班不可见、version 递增
  - 实测：`internal/store/skills.go` 实现；handler 单测 `TestDeleteSkillCrossClass404` 通过；真机见第 6 节
- [x] 4.3 `internal/httpapi`：技能三路由 + 教师角色 + 跨班 404 + 学生 403
  - verify: 单测覆盖 200/403/404；统一错误体
  - 实测：`TestListSkillsStudentOK` / `TestUpsertSkillStudentForbidden`(403) / `TestDeleteSkillCrossClass404` / `TestUpsertSkillRejectsBadToolRef`(400) 全通过
- [x] 4.4 抽取式降级路径也体现技能引导差异；`meta.skill` 字段可见
  - verify: 零密钥环境开/关技能同一问题两次 `meta.skill.enabled` 不同
  - 实测：`skill.DecorateFallback` 单测通过；`ask.go` 已把 `skill`/`tool_calls` 注入 meta；真机开关对比见第 6 节

## 5. MCP 宿主侧

- [x] 5.1 `internal/mcpclient`：JSON-RPC 客户端（initialize / tools/list / tools/call），令牌走请求头
  - verify: 单测用 httptest 桩覆盖三方法与令牌错误
  - 实测：`go test ./internal/mcpclient` 通过 6 用例，含错令牌、工具声明、参数校验
- [x] 5.2 `internal/store` + `internal/httpapi`：服务器登记/列表/连接/断开/工具清单，令牌只回布尔，class_id 隔离
  - verify: 连接两次清单不翻倍；列表响应搜不到令牌；跨班 404、学生 403
  - 实测：`TestListMCPServersNeverReturnsToken`（搜不到 `SECRET_TOKEN` 与 `auth_token`）/ `TestMCPToolsCrossClass404` / `TestStudentCannotListMCPCalls`(403) 通过；连接替换清单逻辑在 `SetMCPConnected`
- [x] 5.3 工具引用保存前校验：格式、工具在发现清单内、服务器已连接
  - verify: 三种非法引用各返回 400 可读错误且不落库
  - 实测：`mcpclient.ValidateRefs` 单测 + `TestUpsertSkillRejectsBadToolRef`(400 `bad_tool_ref`) 通过
- [x] 5.4 调用编排：参数校验、结果检查、轮次/累计上限与优雅降级
  - verify: 单测覆盖成功/参数被拒/指令丢弃/超限降级四条
  - 实测：`TestExecuteOK` / `TestValidateArguments` / `TestExecuteFiltersInstruction` / `TestWithinLimits` 通过
- [x] 5.5 `mcp_tool_calls` 只增不改，五问字段齐全，教师本班可读、默认 20 上限 50
  - verify: 三种结果各一条；学生 403、非法 limit 400、跨班 404
  - 实测：`TestListMCPCallsInvalidLimit`(abc/0/-1 → 400) / `TestMCPCallsHaveFiveFields` 通过
- [x] 5.6 SSE `meta.tool_calls` 在正文前可见
  - verify: 真机抓 SSE 首帧含 tool_calls（真机见第 6 节）
  - 实测：`ask.go` 在首个 meta 事件写入 `tool_calls`；apitest 端到端用例抓首帧核对

## 6. Swagger、apitest 与门禁

- [x] 6.1 OpenAPI 描述覆盖新增 8 路由；浏览器可打开 Swagger 试调
  - verify: openapi.json 含全部新路径，错误码与实现一致
  - 实测：`frontend/public/openapi.json` 16 条路径、JSON 合法；`/swagger.html` 用 CDN Swagger UI，含离线降级提示；两者经 Vite 复制进 `backend/internal/web/dist`
- [x] 6.2 `apitest/`：按 URL 组织脚本 + `run_all.py` 生成 HTML 报告，含端到端调用断言
  - verify: 一键运行全绿，报告存在且不含密钥（真机见第 6 节）
  - 实测：`python -m py_compile` 全部 apitest 脚本通过；`run_all.py` 汇总 `check()` 并写 `apitest/report.html`（已 gitignore）
- [x] 6.3 全量门禁：`gofmt` / `go build` / `go vet` / `go test` / 分层 / 密钥 / 前端 build / strict
  - verify: `GO_BIN=... node scripts/quality-gate.mjs --with-spec` 全绿
  - 实测：`node scripts/quality-gate.mjs` **7 项全绿、GATE=0**；`tsc -b && vite build` 38 modules 通过；`validate --all --strict` **14 passed / 0 failed**
- [ ] 6.4 真机四/五服务重启后验证技能开关与 MCP 端到端；记录命令与原始输出
  - verify: `/health` 200；技能开/关行为差异可见；MCP 调用记录含 tool@server
  - **未实测**：本机 Docker daemon 未运行（`docker compose ps` 报 `npipe ... dockerDesktopLinuxEngine ... not found`），四/五服务无法启动。mcp-server 已单独本地起进程验证（见第 5 节），但主应用 + MySQL + Qdrant 联动的真机验证待 Docker 启动后补跑 `python apitest/run_all.py`

## 7. 文档

- [x] 7.1 README 增加技能与 MCP 的使用说明、接口清单、验证命令
  - verify: 文档与实现一致
  - 实测：README「五项 AI 能力」表更新为全部 ✅；新增「技能与 MCP 的使用与验证」小节；本地运行段更新为五服务
- [x] 7.2 `.env.example` 增加 `MCP_AUTH_TOKEN` 键名占位，明文只在 `deploy/.env`
  - verify: `check-secrets.mjs` 通过，全仓无令牌明文
  - 实测：`✓ 密钥/配置卫生检查通过`；本机 `deploy/.env` 已补随机令牌且经 `git check-ignore` 确认不入库

## 5. 真机验证记录（mcp-server 独立进程）

### 5.1 mcp-server HTTP 真机（本地起进程，令牌 `localtesttoken123`）

命令：`MCP_AUTH_TOKEN=localtesttoken123 MCP_ADDR=:18090 ./mcp-server.exe`，随后 curl：

| 请求 | 结果 |
| --- | --- |
| `GET /health` | `{"status":"ok","service":"mcp-server","version":"dev"}` |
| `POST /mcp` `tools/list` 无令牌 | `{"error":{"code":-32001,"message":"invalid_token"}}` |
| `initialize` 带令牌 | `protocolVersion=2024-11-05`，`serverInfo.name=campus-tools`，`capabilities.tools` |
| `tools/list` 带令牌 | 2 个工具：`formula_hint`、`unit_convert`，每个含 name/description/inputSchema |
| `tools/call formula_hint {"topic":"酶"}` | 返回分步线索纯文本，`isError=false` |
| `tools/call unit_convert {"value":2,"from":"km","to":"m"}` | `2000 m（按 1 km = 1000 m 换算）` |
| 未知方法 `bogus` | `{"error":{"code":-32601,"message":"method_not_found"}}` |

服务已停止（按端口 PID `taskkill` 后 `netstat` 确认无监听）。

## 6. 未验证边界（诚实清单）

- **五服务联动真机**：Docker daemon 未运行，主应用 + MySQL + Qdrant + worker + mcp-server 的联动未跑；apitest 与技能开关/工具端到端需 Docker 启动后补 `python apitest/run_all.py`。
- **Swagger UI 在线试调**：依赖公共 CDN；本机未开浏览器验证在线加载，离线降级路径已写入页面。
- **同伴交叉验证**（MC2 四项：服务隔离/令牌不回传/断开即失效/记录只看自己范围）：需另一位成员以自己账号复现，属 [队] 项，本轮未做。
- **三份真实用户反馈、5 分钟汇报、录屏**：属课程第 10/11 课交付物，不在本 change 范围。
