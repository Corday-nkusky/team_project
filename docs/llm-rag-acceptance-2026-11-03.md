# PR #4 LLM / Embedding / RAG 验收记录（2026-11-03）

## 环境

- 分支：`feat/llm-gateway`
- 网关：课程网关 `/v1`（密钥仅存本机 `deploy/.env`，本文不记录）
- Chat Model：`course-chat`
- Embedding Model：`course-embedding`
- 期望维度：2048
- 本地验收地址：`http://127.0.0.1:18080`（正式 8080 被另一项目占用）

## 直接网关验证

| 项目 | 结果 | 证据 |
| --- | --- | --- |
| 批量 Embedding | PASS | 一次请求 2 条文本，返回 2 条向量 |
| Embedding 维度 | PASS | 两条向量均为 2048 维 |
| Chat Streaming | PASS | HTTP 200，`text/event-stream`，23 个 delta 帧，收到 `[DONE]` |

## 应用健康检查

`GET /health` 返回 HTTP 200：

- `deps.mysql=ok`
- `deps.qdrant=ok`
- `llm.enabled=true`
- `llm.chat_model=course-chat`
- `llm.embed_model=course-embedding`
- 响应不含 API key

## 完整链路

执行：

```bash
BASE_URL=http://127.0.0.1:18080 node scripts/verify-llm.mjs
```

结果：退出码 0，全部通过。

验证步骤与结果：

1. 教师和学生登录成功。
2. 教师上传独有词材料，HTTP 201。
3. 材料状态为 `ready`。
4. Embedding 成功并写入 Qdrant。
5. 学生问题返回 `text/event-stream`。
6. 未出现 `no_evidence`、`degraded` 或 `error`。
7. 收到 15 个 delta 与 done。
8. 回答包含独有验证词。
9. 引用清单非空，引用指向本次材料，引用片段包含验证词。

## Qdrant 核验

- 集合：`team_campusclaw_chunks`
- 集合状态：green
- 向量维度：2048
- 距离算法：Cosine
- 验证材料 ID：7
- 材料向量点数：1
- payload：`class_id=1`、`material_id=7`、`chunk_id=14155`
- 真实问题向量搜索：HTTP 200，以相似度 `0.60979766` 召回材料 7。
- 对材料 7 再次调用重索引：HTTP 200、状态 `ready`、无错误；重索引后点数仍为 1，没有重复向量。

## 结论

- Chat Model：PASS
- Streaming：PASS
- Embedding：PASS
- 2048 维度：PASS
- Qdrant 集合创建：PASS
- 向量写入：PASS
- 向量检索：PASS
- 材料关联过滤：PASS
- 重索引幂等性：PASS
- 上传 → 切片 → Embedding → Qdrant → 提问 → 检索 → LLM → 引用完整 RAG：PASS

## 浏览器回归遗留

启用真实 LLM 后执行 `BASE=http://127.0.0.1:18080 node scripts/e2e-ui.mjs`：47 项通过、4 项失败，退出码 1。失败项集中在 headless Chrome/CDP 合成键盘事件与焦点断言：

- T3e：Esc 关闭阅读面板后焦点回归；
- T3g：Enter 打开阅读面板后关闭按钮焦点；
- T5c：Enter 展开引用；
- T5d：Space 收起引用并保持可见焦点。

同一轮中的真实上传、全文读取、LLM 流式回答、引用关联、无依据拒答、权限隔离和窄屏布局均通过。脚本源码已经说明 headless Chrome 的 CDP 键盘事件存在未派发情况，因此目前不能仅凭该结果断定产品键盘行为回归，也不能把浏览器 E2E 标记为 PASS；需要用可观察 keydown 计数和真实前台 Chrome/辅助技术单独定位。

未在本轮执行破坏性维度迁移测试，未删除集合、数据库或数据卷。Embedding 故障状态由现有自动化测试覆盖；若需真实故障演练，应使用独立数据库/集合，避免在共享验收数据中留下故障材料。
