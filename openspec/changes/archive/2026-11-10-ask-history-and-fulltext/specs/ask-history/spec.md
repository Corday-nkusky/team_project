# Spec Delta

## ADDED Requirements

### Requirement: H1 历史问答只读查询

系统 SHALL 提供需要登录的 `GET /api/conversations`，按「发起者本人 + 发起者所在班」返回已归档的完整问答，字段含记录 id、问题、回答、引用清单与创建时间，按 `id DESC` 排序且条数可限。`limit` SHALL 默认 20、上限 50；非正整数或无法解析 SHALL 返回 400 且不查库，超过上限 SHALL 截断而非报错。系统 SHALL NOT 返回其他用户或其他班级的记录，SHALL NOT 把中断、空回答或未命中材料的问答写入历史。

#### Scenario: H1.1 本人本班历史可完整回看

- **GIVEN** 用户 A 属于班级 1，已产生两条完整问答（含各自的引用清单）；同班用户 B 与班级 2 也各有问答
- **WHEN** A 请求 `GET /api/conversations`
- **THEN** 返回 200，仅含 A 在班级 1 的两条记录，按 `id DESC` 排列
- **AND** 每条记录的问题、回答全文、引用清单与创建时间与落库时一致

#### Scenario: H1.2 条数上限与空列表

- **GIVEN** 用户 A 有 60 条历史；用户 C 从未提问
- **WHEN** A 请求 `limit=100`，C 请求不带 `limit`
- **THEN** A 得到 50 条（上限截断），C 得到 `{"items":[]}` 而非 `null` 或 500
- **AND** 两者都只含本人本班数据

#### Scenario: H1.3 失败路径：未认证与非法参数

- **GIVEN** 服务已启动，历史接口可用
- **WHEN** 不带 `Authorization` 请求，或携带合法令牌请求 `limit=abc` / `limit=0` / `limit=-1`
- **THEN** 前者返回 401，后者返回 400，均使用统一错误体且不执行历史查询

#### Scenario: H1.4 失败路径：查询失败不伪装为空

- **GIVEN** 历史查询所依赖的数据库不可用
- **WHEN** 已登录用户请求历史列表
- **THEN** 返回 5xx 与统一错误体，界面显示可读错误与重试入口
- **AND** 不返回 `{"items":[]}` 假装「没有历史」

### Requirement: H2 历史问答在工作台可回看

提问工作台 SHALL 提供历史问答面板，列出问题摘要与时间，用户 SHALL 能展开查看**完整回答**与当时的引用清单；展开的历史引用 SHALL 与即时回答的引用一样可打开整篇材料全文。面板 SHALL 覆盖加载中、读取失败、无记录与正常四态，失败态提供重试。历史面板 SHALL NOT 混入其他标签页或其他班级内容。

#### Scenario: H2.1 展开一条历史问答

- **GIVEN** 用户已登录且有一条含 2 条引用的完整历史问答
- **WHEN** 打开提问工作台并展开该条历史
- **THEN** 显示当时的问题、完整回答与 2 条引用（材料标题与切片原文）
- **AND** 引用条目的「阅读全文」可打开对应材料整篇原文

#### Scenario: H2.2 新完成的问答出现在历史里

- **GIVEN** 用户刚收到一次问答的 `done` 事件且 `conversation_id` 非 null
- **WHEN** 历史面板刷新
- **THEN** 该问答出现在列表首条，内容与刚才流式收到的回答一致

#### Scenario: H2.3 失败路径：空态与错误态

- **GIVEN** 用户从未提问，或历史接口返回 5xx
- **WHEN** 打开历史面板
- **THEN** 前者显示「还没有历史问答」，后者显示错误原因与「重试」
- **AND** 两者都不渲染空白面板，也不把加载中误显示为空态

### Requirement: H3 结束事件携带归档标识

完整回答的 `done` 事件 SHALL 携带 `conversation_id`，其值与刚落库的历史记录 id 一致；落库失败 SHALL 仍发送 `done` 且该字段为 `null`，只记 ERROR 日志。未命中材料、生成中断、空回答与客户端断开 SHALL NOT 产生历史记录（沿用 R6.3 / R6.4），`no_evidence` 分支的 `done` 保持 `conversation_id: null`。

#### Scenario: H3.1 完整回答返回真实记录 id

- **GIVEN** 本班材料可命中且回答完整生成
- **WHEN** 客户端收到 `done`
- **THEN** 事件的 `conversation_id` 为正整数，`GET /api/conversations` 中能找到同 id 且回答一致

#### Scenario: H3.2 失败路径：落库失败不影响回答

- **GIVEN** 回答已完整生成，但会话写入失败
- **WHEN** 客户端收到 `done`
- **THEN** 仍收到 `done`（不是 `error`），`conversation_id` 为 `null`
- **AND** 服务端留下 ERROR 日志，历史列表中不出现本次问答

#### Scenario: H3.3 失败路径：未完成问答不归档

- **GIVEN** 一次问答被客户端中断、模型中断或回答为空
- **WHEN** 流结束后请求历史列表
- **THEN** 历史条数与问答前一致，本次问答不出现
