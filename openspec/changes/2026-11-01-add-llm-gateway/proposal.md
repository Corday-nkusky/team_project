# Proposal

## Why

代码已经具备调用单一 OpenAI 兼容网关的能力，但主规格只规定「`LLM_API_KEY` 为空时走抽取式降级」，没有规定启用后的网关契约、模型名来源、健康信息与向量索引语义。实际接入时会出现四类无法验收的情况：

- 只填 key 与 base，依赖代码里的 DeepSeek 默认模型名，静默调用错误模型；
- 接上只支持对话、不支持 `/embeddings` 的网关，上传时所有材料变成 `failed`；
- 换嵌入模型或维度后继续使用旧集合，索引失败原因只剩一条底层错误；
- 运维无法从 `/health` 判断当前启用的到底是哪两个模型。

价值三问：

- **谁的问题**：需要模型生成回答的学生、上传后必须看到材料 `ready` 的教师，以及配置模型网关的维护者。
- **不做会怎样**：模型路径无法复现验收；错误配置以业务层的材料 `failed` 出现；更换模型或维度时没有可操作契约。
- **做完如何验证**：在同时支持 `/chat/completions`（SSE）与 `/embeddings` 的网关上，`/health` 显示 `enabled` 与两个模型名；上传材料 `ready` 且存在向量；学生提问得到流式回答与引用；嵌入失败时材料 `failed` 且带原因，恢复后重索引转 `ready`。

## What Changes

1. 新增 `llm-gateway` capability，规定网关配置、健康信息、对话与嵌入调用、向量索引与重索引语义。
2. 启用模型网关时，`LLM_API_BASE`、`LLM_CHAT_MODEL`、`LLM_EMBED_MODEL` 必须显式配置；删除代码中的模型名默认值，缺失时在监听端口前退出。
3. `GET /health` 在启用时增加 `llm.chat_model` 与 `llm.embed_model`，不暴露 key、base 或连接串。
4. 增加真实网关集成测试与本地全链路验证；长材料嵌入与向量写入分批，换嵌入维度时在删除旧向量前失败；更新 `.env.example`、compose 与 README。
5. 零密钥降级、问答、流式与引用行为保持不变。

## Capabilities

### New Capabilities

- `llm-gateway`：一个兼容网关同时提供流式对话与嵌入时的配置、健康、索引与失败契约。

### Modified Capabilities

（无；现有 R5/R6/R7 的检索、流式与引用行为不变，材料索引失败仍按 R3.3 处理。）

## Non-goals（本次不做）

| 项 | 原因 |
| --- | --- |
| 对话与嵌入使用不同网关、不同密钥 | 当前网关同时支持两类端点；拆分需要另一套配置、健康与密钥边界，另开 change |
| 模型自动切换、负载均衡、重试与熔断 | 会改变“不静默换模型”的语义，属于独立的可靠性设计 |
| 自动迁移或重建 Qdrant 集合 | 维度迁移涉及数据删除与重索引窗口；本次只要求失败可见并写明操作步骤 |
| 前端模型选择或管理界面 | 模型是部署配置，不向终端用户暴露 |
| 修改问答依据判定、SSE 协议或引用规则 | 由既有 R5/R6/R7 与 hardening change 负责 |
| 把真实 key、base 或模型凭据写入仓库/镜像 | 继续只从本机 `deploy/.env` 注入 |

## Impact

- **后端**：`internal/config`（模型名必填校验）、`internal/httpapi`（`/health` 字段）、`internal/llm`（嵌入分批、去重与集成测试）、`internal/qdrant`（集合维度核对、向量分批写入）；`internal/qa` 的索引编排不变。
- **配置**：`deploy/.env.example`、`deploy/docker-compose.yml` 去除模型名默认值；本机 `deploy/.env` 只写真实值，不入库。
- **验收**：新增带 build tag 的真实网关集成测试；本地执行上传、索引、问答与 Qdrant 点核对；嵌入失败与恢复演练。
- **文档**：README 的模型网关说明、`docs/project-status-guide.html` 的状态与验证计数。
- **兼容**：`LLM_API_KEY` 为空时行为不变；启用时必须补全两个模型名，这是有意的启动期失败。
