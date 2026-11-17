# Proposal: 历史问答可查与引用阅读全文

## Why

- 后端在 `conversations` 表里已保存每次**完整**问答（R6.3 保证中断与空回答不落库），但没有任何读取接口，用户无法看到自己问过什么、当时答了什么、依据哪几条材料。数据一直在写、从来读不出来。
- 回答的引用清单只给出**命中切片**的完整原文。切片是整篇材料的一小段，用户核对上下文时仍要跳到材料页手动找同一篇；侧栏还写着「引用可回到原文」，实际做不到。

## What Changes

- 新增 `GET /api/conversations`：需要登录，按「本人 + 本班」过滤，倒序返回完整问答与引用清单，条数可限且有上限。
- SSE 的 `done` 事件带上真实 `conversation_id`（落库失败时为 `null`），让客户端能确认本次问答已归档。
- 提问工作台新增历史问答面板：可展开查看完整回答与当时的引用，并可继续打开材料全文。
- 引用条目新增「阅读全文」：复用既有 `GET /api/materials/{id}` 与阅读面板，打开**整篇材料原文**，并定位高亮本次命中的切片。
- 把材料页的阅读面板抽成共用组件，材料页与引用入口使用同一套加载 / 错误 / 空 / 正常四态、Esc 关闭与焦点恢复行为，不另造一份。

## Capabilities

### New Capabilities

- `ask-history`：历史问答的查询契约、隔离、状态与界面回看。

### Modified Capabilities

- `knowledge-retrieval`：R7 增加「引用可打开整篇材料全文」场景。
- `material-reading`：MR2 增加「从回答引用打开阅读面板并定位切片」场景。

## Impact

- 后端：`internal/store` 增加只读查询，`internal/httpapi` 增加路由与 handler，`SaveConversation` 改为返回记录 id；不改表结构，存量会话原样可读。
- 前端：`api.ts` 增加类型与列表函数；新增 `components/MaterialReader.tsx`；`AskPage.tsx` 增加历史面板与全文入口；`MaterialsPage.tsx` 改用共用组件；`styles.css` 增加历史面板与高亮样式。
- 不改：检索链路、索引链路、模型配置、部署编排、演示数据。
