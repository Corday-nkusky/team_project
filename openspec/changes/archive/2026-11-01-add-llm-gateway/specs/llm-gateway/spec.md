# Spec Delta

## Purpose

让维护者能用一组声明的 OpenAI 兼容网关同时提供流式对话与嵌入，并让启用后的模型、向量索引与失败路径可判定、可复现。未配置密钥时保持既有的抽取式降级。

## ADDED Requirements

### Requirement: LG1 网关配置与健康

系统 SHALL 从环境变量读取 `LLM_API_BASE`、`LLM_API_KEY`、`LLM_CHAT_MODEL`、`LLM_EMBED_MODEL`。`LLM_API_KEY` 为空时 SHALL 保持抽取式降级，且不要求其余三项。密钥非空时，`LLM_API_BASE` SHALL 是 http(s) 绝对地址，`LLM_CHAT_MODEL` 与 `LLM_EMBED_MODEL` SHALL 非空；任一缺失或非法时进程 SHALL 在监听端口前以非零码退出，日志 SHALL 只指明键名，不得回显该键的值。系统 SHALL NOT 为模型名提供内置默认值。`GET /health` SHALL 含 `llm.enabled`；启用时 SHALL 额外含 `llm.chat_model` 与 `llm.embed_model`；SHALL NOT 含 `LLM_API_KEY`、`LLM_API_BASE` 或任何连接串。

#### Scenario: LG1.1 启用后健康信息可核对

- **GIVEN** 四项配置齐全且合法，网关可用
- **WHEN** 启动服务并请求 `GET /health`
- **THEN** 返回 200，`llm.enabled` 为 `true`
- **AND** `llm.chat_model` 与 `llm.embed_model` 与配置值一致
- **AND** 响应体不含 key、base 或密钥形态的字符串

#### Scenario: LG1.2 缺模型名拒绝启动

- **GIVEN** `LLM_API_KEY` 与 `LLM_API_BASE` 已设置，`LLM_CHAT_MODEL` 或 `LLM_EMBED_MODEL` 为空
- **WHEN** 启动服务
- **THEN** 进程以非零码退出，且未进入监听状态
- **AND** 日志含缺失的键名，不含 key 的值

#### Scenario: LG1.3 空密钥保持降级

- **GIVEN** `LLM_API_KEY` 为空
- **WHEN** 启动服务并请求 `GET /health`
- **THEN** 进程正常监听，`llm.enabled` 为 `false`
- **AND** 不要求 `LLM_API_BASE` 或模型名
- **AND** 问答仍走既有抽取式降级（R8.6）

### Requirement: LG2 网关调用与向量索引

启用后，对话 SHALL 调用 `{LLM_API_BASE}/chat/completions`，使用 `LLM_CHAT_MODEL` 且 `stream=true`，按 SSE 增量读取直至 `[DONE]`；嵌入 SHALL 调用 `{LLM_API_BASE}/embeddings`，使用 `LLM_EMBED_MODEL` 并分批提交该材料的全部切片，相同切片可去重，任一批失败即整体失败。材料 SHALL 仅在切片落库、嵌入成功、旧向量删除与新向量写入全部完成后标记为 `ready`；嵌入或向量写入失败 SHALL 标记为 `failed` 并在 `index_error` 中保留原因，重索引 SHALL 可重试。向量维度 SHALL 取网关返回的实际维度，不同批次返回的维度 SHALL 一致；集合已存在时 SHALL 在删除旧向量前核对维度，不一致 SHALL 立即失败，且 SHALL NOT 混用不同维度、SHALL NOT 把维度冲突静默降级为仅关键词成功。

#### Scenario: LG2.1 真实网关完成上传到引用

- **GIVEN** 网关同时支持 `/chat/completions`（SSE）与 `/embeddings`，后端已启用模型
- **WHEN** 教师上传一份含独有词的材料并等待索引，学生就该材料提问
- **THEN** 材料 `index_status` 为 `ready`，且 Qdrant 中存在该材料的向量点
- **AND** 回答以 SSE 增量返回，含至少一条 `[n]` 引用，引用标题为该材料标题
- **AND** 不出现 `no_evidence` 或 `degraded`

#### Scenario: LG2.2 嵌入失败可察觉可恢复

- **GIVEN** 网关已启用，但 `/embeddings` 返回错误
- **WHEN** 教师上传材料
- **THEN** 材料 `index_status` 为 `failed`，`index_error` 含嵌入失败信息
- **AND** 该材料不进入 `ready` 检索集
- **AND** 网关恢复后调用重索引，材料转为 `ready` 且向量可查

#### Scenario: LG2.3 向量维度不混用

- **GIVEN** Qdrant 集合已按维度 D 创建
- **WHEN** 当前嵌入模型返回维度 D'≠D 并尝试建立索引
- **THEN** 索引失败，材料标记为 `failed`，错误可见
- **AND** 在删除该材料的旧向量之前就失败，不写入维度不一致的向量
- **AND** 不把该次索引报告为成功或仅关键词成功
