# Design: 历史问答可查与引用阅读全文

## 现状（先核前提，避免重造）

| 已有能力 | 位置 | 本轮用法 |
| --- | --- | --- |
| `conversations(user_id, class_id, question, answer, citations, created_at)` | `db.go` schema | **不新建表**，只补只读查询 |
| `SaveConversation` 仅在完整回答后调用 | `httpapi/ask.go` `finishAsk` | 改为返回 `LastInsertId`，写进 `done` |
| `GET /api/materials/{id}` 返回完整正文，跨班 404 | MR1 / `handleGetMaterial` | 引用「阅读全文」直接复用 |
| 阅读面板四态 / Esc / 焦点恢复 | `MaterialsPage.tsx` | 抽成 `MaterialReader` 共用 |

## 决策

1. **只读一个列表接口，不做详情接口**：一次问答的回答通常几百字，20 条上限内直接返回 `answer`、`citations` 全文，避免为「点开一条」再发一次请求；引用 JSON 原样透传，不重新检索。
2. **隔离用 `user_id + class_id` 双条件**：历史是「我的问答」，不是全班问答墙；任何业务查询都带 `class_id`（AGENTS.md 第五节）。排序 `id DESC` 带唯一 tiebreaker。
3. **`limit` 默认 20、上限 50**：非正整数或无法解析 → 400；超过 50 截断，不报错，避免一次拉走整表。
4. **落库失败不影响回答**：`SaveConversation` 失败仍发 `done`，`conversation_id: null`，只记 ERROR 日志；与既有「审计失败不回滚回答」一致。
5. **引用全文 = 整篇材料，不是更长摘要**：`snippet` 本来就是命中切片的完整原文（R7.5）。新增的是从该切片回看上下文的能力。阅读面板打开后用 `snippet` 在 `content` 里做一次 `indexOf` 定位并高亮；找不到时仍显示全文，不伪造定位。
6. **阅读面板共用而非复制**：组件自己持有 `open` / 加载 / 错误 / 焦点状态，通过 `ref` 暴露 `open(materialId, snippet?, trigger?)`；材料页删除同名本地状态，DOM 结构与类名保持不变，避免破坏既有 MR2 断言与浏览器 e2e。

## 失败路径

| 情况 | 行为 |
| --- | --- |
| 未登录请求历史 | 401 统一错误体 |
| `limit` 非法 | 400，不查库 |
| 历史查询失败 | 500 统一错误体 + ERROR 日志，界面显示「读取失败 / 重试」 |
| 无历史记录 | `{"items":[]}` 与界面空态，不显示 `null` |
| 材料已删除 / 跨班 | 阅读面板沿用 MR1 的 404 错误态与「重试读取」 |
| 材料正文里找不到该切片 | 显示整篇全文 + 提示「未能定位到引用片段」，不假装高亮 |
| 客户端中断 / 空回答 | 不落库，历史里不出现（既有 R6.3 / R6.4） |

## 兼容性

- 不改任何表结构，`conversations` 存量行（含早期 `citations` 为 NULL 的行）按 `[]` 返回。
- `done` 事件新增字段，旧客户端忽略即可；`no_evidence` 分支仍返回 `conversation_id: null`。
- 前端构建产物由 Go `embed` 内嵌，部署编排与镜像不变。
