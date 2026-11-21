## MODIFIED Requirements

### Requirement: LG2 网关调用与向量索引

启用后，对话 SHALL 调用 `{LLM_API_BASE}/chat/completions`，使用 `LLM_CHAT_MODEL` 且 `stream=true`，按 SSE 增量读取直至 `[DONE]`；嵌入 SHALL 调用 `{LLM_API_BASE}/embeddings`，使用 `LLM_EMBED_MODEL` 并分批提交该材料的全部切片，相同切片可去重，任一批失败即整体失败。材料 SHALL 仅在切片落库、嵌入成功、旧向量删除与新向量写入全部完成后标记为 `ready`；嵌入或向量写入失败 SHALL 标记为 `failed` 并在 `index_error` 中保留原因，重索引 SHALL 可重试。系统 SHALL 使用固定集合 `team_campusclaw_chunks` 读写向量。向量维度 SHALL 取网关返回的实际维度，不同批次返回的维度 SHALL 一致；集合已存在时 SHALL 在删除旧向量前核对维度，不一致 SHALL 立即失败，且 SHALL NOT 混用不同维度、SHALL NOT 把维度冲突静默降级为仅关键词成功。

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

- **GIVEN** 原集合已按维度 D 建立，当前模型返回的实际维度为 D'≠D
- **WHEN** 实际维度为 D' 的嵌入向目标集合建立索引
- **THEN** 索引失败，材料标记为 `failed`，错误可见
- **AND** 在删除该材料的旧向量之前就失败，不写入维度不一致的向量
- **AND** 不把该次索引报告为成功或仅关键词成功

#### Scenario: 复用原集合中的向量

- **GIVEN** 原集合已有本班材料的向量，嵌入配置保持一致
- **WHEN** 升级后学生就该材料提问
- **THEN** 系统查询原集合，并可返回标有向量来源的引用

## REMOVED Requirements

### Requirement: LG4 向量空间隔离

**Reason**: 本次部署需要复用存量向量，延后集合拆分。

**Migration**: 沿用原集合和嵌入配置即可升级。今后更换嵌入配置时按原流程重建索引。
