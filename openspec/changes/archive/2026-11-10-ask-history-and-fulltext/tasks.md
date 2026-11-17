# Tasks: 历史问答可查与引用阅读全文

> 每条已补实测（命令 + 结果），未实测边界在第 5 节如实标注。

## 1. 规约先行

- [x] 1.1 新建 change 四件套（proposal / design / 三份 spec delta / tasks），Scenario 三段式且每个 Requirement 含失败路径
  - verify: `npx @fission-ai/openspec@latest validate --all --strict` 通过
  - 实测：`11 passed / 0 failed`（含新 change 与 7 份 spec）
- [x] 1.2 主规约同步：新增 `ask-history`，更新 `knowledge-retrieval` R7 与 `material-reading` MR2 的正文、场景与对照表
  - verify: 主规约与 delta 逐字一致，strict 通过
  - 实测：主规约 R7 追加 R7.6/R7.7、MR2 追加 MR2.5/MR2.6；`auth-upload` / `knowledge-retrieval` 对照表与新增 `ask-history` 对照表同步

## 2. 后端：历史只读查询与归档标识

- [x] 2.1 `store` 增加 `ListConversationsJSON`（`user_id + class_id`、`id DESC`、`limit` 上限 50、NULL citations 归一为 `[]`）
  - verify: `go test ./internal/store` 通过；真机 SQL 不返回他人/他班记录
  - 实测：真机 `GET /api/conversations?limit=100` 返回 49 条（teacher1 本人本班共 49 条），不含 user_id=2 的 71 条
- [x] 2.2 `SaveConversation` 返回插入 id；`done` 事件带 `conversation_id`，落库失败仍发 done 且为 null
  - verify: `go test ./internal/httpapi` 覆盖成功 / 落库失败两条分支
  - 实测：`TestAskDoneCarriesConversationID` / `TestAskDoneIsNullWhenSaveFails` / `TestAskNoEvidenceHasNoConversationID` 通过；真机 `done.conversation_id=137`
- [x] 2.3 新增 `GET /api/conversations`（登录校验、limit 校验、统一错误体、空列表返回 `[]`）
  - verify: 单测覆盖 200 / 401 / 400 / 空列表 / 查询失败；真机 curl 验证
  - 实测：真机 200（3 条、按 id 倒序）、`?limit=abc|0|-1` → 400、无 Authorization → 401；`TestListConversationsEmptyIsArray` 验证 `{"items":[]}`
- [x] 2.4 补单测：隔离、排序、上限、空列表、非法参数、未认证
  - verify: `go test ./...` 全绿
  - 实测：新增 `backend/internal/httpapi/conversations_test.go` 8 个用例，`go test ./...` 全通过

## 3. 前端：引用全文与历史面板

- [x] 3.1 抽出 `components/MaterialReader.tsx`（四态、Esc、焦点恢复、可选切片高亮定位），材料页改用同一组件
  - verify: `npm run build` 通过；材料页 DOM 类名不变
  - 实测：`tsc -b && vite build` 通过（37 modules）；定向验收 U5 材料页阅读面板仍可打开全文，U4 Esc + 焦点归还通过
- [x] 3.2 AskPage 每条引用加「阅读全文」，打开整篇原文并定位命中切片；历史引用同样可用
  - verify: 浏览器真机点击后全文可见、切片高亮；材料不存在时显示错误态
  - 实测：U3 历史引用全文 2400 字 / 高亮切片 342 字；U6b 即时回答引用同样打开全文并高亮
- [x] 3.3 AskPage 加历史问答面板（列表 / 展开完整回答与引用 / 空态 / 失败重试 / 新问答后刷新）
  - verify: 浏览器真机可见历史记录，展开内容与接口一致
  - 实测：U1 历史面板列出 20 条真实记录且无错误态；U2 展开显示完整回答与当时的引用；真实问答后 `done` 触发刷新，历史首条 id 与回执一致
- [x] 3.4 `api.ts` 增加类型、`listConversations`、`onDone(conversation_id)`；`styles.css` 增加历史面板与高亮样式
  - verify: `npm run build` 通过，产物包含新文案
  - 实测：构建通过，产物 `index-CLN9x4VZ.js` 含「历史问答」「阅读全文」；`history-card` / `material-hit` 样式生效

## 4. 门禁与真机验收

- [x] 4.1 `gofmt` / `go build` / `go vet` / `go test` / 前端 build / 质量门禁 / OpenSpec strict
  - verify: `GO_BIN=... node scripts/quality-gate.mjs --with-spec` 全绿
  - 实测：门禁 8 项全绿；strict `11 passed / 0 failed`
- [x] 4.2 重启四服务后真机验证：历史接口、done 的 conversation_id、引用全文接口、浏览器界面
  - verify: 保留命令与原始输出，记录到本节与项目日志
  - 实测：见第 5 节；`/health` 200（glm-5.3-flash / embedding-3），定向验收 `8 通过 / 0 失败`
- [x] 4.3 文档同步（README 接口清单 / 使用说明）
  - verify: README 与实际接口一致
  - 实测：README 新增「历史问答与引用全文」小节，含接口、隔离、done 回执、阅读面板与定向验收命令

## 5. 实测记录

### 5.1 离线

- `gofmt -l .` 无输出；`go build ./...`、`go vet ./...`、`go test ./...` 全通过。
- `cd frontend && npm run build` → `37 modules transformed`，`dist` 产物生成。
- `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` → 8 项全绿。
- `npx @fission-ai/openspec@latest validate --all --strict` → `11 passed / 0 failed`。

### 5.2 真机（四服务重建后，MySQL/Qdrant 数据卷保留）

- `/health` 200：`llm.chat_model=glm-5.3-flash`、`llm.embed_model=embedding-3`、`deps.mysql/qdrant=ok`。
- 登录 `teacher1`（user_id=1 / class_id=1）：
  - `GET /api/conversations?limit=3` → 200，3 条，按 id 倒序；字段 `id/question/answer/citations/created_at`；首条回答 150 字、4 条引用。
  - `?limit=100` → 49 条（本人本班共 49 条，上限 50 未触发）；另 71 条属于 user_id=2，未出现。
  - `?limit=abc` / `?limit=0` / `?limit=-1` → 400；无 `Authorization` → 401。
  - 真实提问「酶的化学本质是什么？请用一句话回答。」→ 32 帧 delta、**未降级**、4 条引用、`done.conversation_id=137`；随后历史首条 `id=137` 且问题一致。
  - `GET /api/materials/{引用 material_id}` → 200，全文 2400 字 / 切片 342 字，`snippet in content = true`（高亮可定位）。
- `node scripts/verify-history-ui.mjs` → **8 通过 / 0 失败**：U0 登录、U1 历史面板 20 条、U2 展开完整回答与引用、U3 全文 2400 字 + 高亮 342 字、U4 Esc 关闭且焦点归还、U5 材料页共用面板、U6a/U6b 即时回答引用全文入口。脚本**不上传材料、不产生测试文本**（问答走页面内 fetch 桩，引用取自真实历史）。
- `停止项目.cmd` 的 `down` 分支实测：4 个容器全部移除，`deploy_mysql-data` / `deploy_qdrant-data` 卷保留（见项目日志）。

### 5.3 未验证边界（如实）

- 浏览器全量 `scripts/e2e-ui.mjs`（51 项）与 Go `cmd/e2e`（81 项）本轮**未重跑**：两者都会上传测试材料，与「清理材料库」冲突；改跑第 5.2 节的不造数据定向验收。
- 历史接口数据库故障分支由单测 `stubRec.listErr` 覆盖（`TestListConversationsQueryFailureIsNotDisguisedAsEmpty`），未制造真实 MySQL 故障。
- `R7.7`（材料已删除）与 `MR2.6`（正文中找不到切片）的界面态由共用组件的错误态 / 定位提示代码实现，未通过删除真实材料复现。
- 同伴评审（DoD 最后一项）未执行，按项目规约需由另一位成员完成。
