# async-indexing Specification

## Purpose

把「上传 / 重索引 → 建索引」从请求内同步执行改为持久化任务驱动的异步链路：提交只写任务，消费者独立执行，结果凭任务标识查询。至少一次投递、幂等、失败分类、班级隔离与三条验收判据构成本能力的可验收边界。

## Requirements

### Requirement: A1 任务记录与事务性发件箱

系统 SHALL 用一张持久化的 `index_tasks` 表记录每次索引工作，含任务标识、关联材料、班级、类型、状态、执行次数、幂等键、失败原因与时间戳。上传 SHALL 把材料行与任务行写入**同一个数据库事务**；事务提交成功则必有任务，回滚则两者都不留。迁移 SHALL 可重复执行，且幂等键落在唯一约束上。

#### Scenario: A1.1 上传在一个事务里写出材料与任务

- **GIVEN** 教师已登录；数据库中记下材料数与任务数
- **WHEN** 上传一份新材料的 SQL 事务提交成功
- **THEN** `materials` 与 `index_tasks` 各新增一行，任务的 `material_id` 指向该材料、`class_id` 与教师所在班一致、`status` 为 `pending`
- **AND** 提交响应尚未返回时，任务已持久化，进程重启后仍可查到

#### Scenario: A1.2 事务回滚不留下孤儿任务

- **GIVEN** 教师在班级 A 上传材料，且任务表插入被数据库约束拒绝
- **WHEN** 该事务回滚
- **THEN** 材料数与任务数都与上传前相同，不存在有任务无材料或有材料无任务的半截状态

#### Scenario: A1.3 迁移幂等与唯一约束

- **GIVEN** 服务已完成一次启动迁移
- **WHEN** 再次执行迁移，并尝试插入同一 `idempotency_key` 的第二条任务
- **THEN** 迁移不报错、表结构与列不重复增加
- **AND** 第二条任务被唯一约束拒绝，任务总数不变

### Requirement: A2 提交契约与只读状态查询

提交类接口 SHALL 返回 `202 Accepted`，响应体只含 `task_id`、`material_id`、`status`、`status_url`，不含切片数、`ready` 或其他索引结果；代码库 SHALL NOT 存在请求内调用 `qa.Index` 的分支。`GET /api/tasks/{task_id}` SHALL 只读返回任务状态、`attempts`、`error` 与时间戳，不触发任何执行。

#### Scenario: A2.1 提交响应只是号码牌

- **GIVEN** 教师已登录且消费者进程已停止
- **WHEN** 上传一份材料
- **THEN** 仍返回 202，响应体含 `task_id` 与以 `/api/tasks/` 开头的 `status_url`
- **AND** 响应体不含 `index_status`、`ready`、`chunk` 或任何索引结果，且任务停留在 `pending`

#### Scenario: A2.2 状态接口只读

- **GIVEN** 本班存在一个 `pending` 任务，且消费者已停止
- **WHEN** 连续查询该任务状态 10 次
- **THEN** 每次都返回 200 与 `pending`，`attempts` 恒为 0，任务未被触发执行

#### Scenario: A2.3 跨班查询不泄漏

- **GIVEN** 班级 B 存在一个任务及其 `task_id`
- **WHEN** 班级 A 的用户查询该 `task_id`
- **THEN** 返回 404，响应体不含该任务的材料、状态或失败原因

### Requirement: A3 状态机与独立消费者

任务状态 SHALL 覆盖 `pending`、`running`、`succeeded`、`failed`、`dead`：领取置 `running` 且 `attempts+1`；成功置 `succeeded` 同时把材料置 `ready`；失败按分类退回 `pending` 或进入终态。消费者 SHALL 是独立于 Web 的执行进程，可单独停止与启动，使用 `FOR UPDATE SKIP LOCKED` 领取、租约超时回收，且执行使用独立 context，不依附 HTTP 请求。

#### Scenario: A3.1 停消费者提交、恢复后自动执行

- **GIVEN** 消费者服务已停止，Web 服务正常
- **WHEN** 上传一份材料并查询任务状态，随后启动消费者
- **THEN** 提交返回 202 且任务停留在 `pending`
- **AND** 消费者启动后自动领取该任务，状态依次可观测到 `running` 与 `succeeded`，无需重新提交

#### Scenario: A3.2 客户端断开不取消索引

- **GIVEN** 消费者正在执行一个索引任务
- **WHEN** 提交该任务的 HTTP 连接被客户端主动断开
- **THEN** 任务仍会到达 `succeeded` 或明确的失败终态，材料中不出现「切片已写、向量未写、状态未知」的静默成功

#### Scenario: A3.3 运行中进程被终止后任务被回收

- **GIVEN** 一个任务处于 `running` 且租约已过期
- **WHEN** 消费者重启
- **THEN** 该任务被重置为 `pending` 并再次执行，`attempts` 不回退，结果不重复

### Requirement: A4 至少一次投递下的幂等

投递语义为**至少一次**。系统 SHALL 在三个位置去重：由业务含义派生并落在唯一约束上的幂等键、`chunks(material_id, seq)` 唯一约束、以及由材料与内容确定性派生的 Qdrant 点 ID。重复提交与重复投递 SHALL 只产生一份结果，`attempts` 可以增加。

#### Scenario: A4.1 重复提交只产生一份结果

- **GIVEN** 教师已登录，本班无同内容材料
- **WHEN** 连续两次上传标题与正文完全相同的材料
- **THEN** 两次都返回 202 且 `task_id` 相同，材料行数与任务行数各只增加 1
- **AND** 轮询至终态后 `chunks` 中该材料每个 `seq` 只有一行

#### Scenario: A4.2 重复投递不改变结果

- **GIVEN** 一个任务已成功写入切片与向量
- **WHEN** 人为把该任务重置为 `pending` 使其被再次投递
- **THEN** `chunks` 行数与 Qdrant 中该材料的点数均不增加，`attempts` 增加，材料仍为 `ready`

#### Scenario: A4.3 幂等键不是随机值

- **GIVEN** 同一份材料内容不变、材料状态不变
- **WHEN** 连续两次调用重索引接口
- **THEN** 两次返回同一个 `task_id`，第二次不新增任务；内容或 `revision` 变化后才允许产生新任务

### Requirement: A5 失败分类、退避与死信

系统 SHALL 区分可重试失败（依赖超时、网络抖动、5xx）、不可重试失败（同样输入必然同结果，如切片为空）与网关额度限流（429）。可重试失败 SHALL 按 `base * 2^(attempts-1)` 叠加随机抖动退避，达到 `WORKER_MAX_ATTEMPTS` 转 `dead` 并保留 `error`；不可重试失败与限流 SHALL 直接进终态，不消耗重试额度。

#### Scenario: A5.1 瞬时失败退避后重试成功

- **GIVEN** 第一次执行因依赖暂时不可用而失败，之后依赖恢复
- **WHEN** 消费者按策略重试该任务
- **THEN** `attempts` 递增、`run_at` 晚于失败时刻，任务最终到达 `succeeded`

#### Scenario: A5.2 达到次数上限转死信

- **GIVEN** 某任务每次执行都因依赖不可用而失败
- **WHEN** `attempts` 达到 `WORKER_MAX_ATTEMPTS`
- **THEN** 任务状态为 `dead`，保留非空 `error` 与 `finished_at`，材料 `index_status` 为 `failed`
- **AND** 该任务不再被领取，队列深度不因它持续增长

#### Scenario: A5.3 不可重试失败不消耗额度

- **GIVEN** 一份材料的输入使索引必然失败（如切片为空）
- **WHEN** 消费者首次执行即判定为 permanent
- **THEN** 任务直接进入 `failed`，`attempts` 为 1，不产生退避重投

#### Scenario: A5.4 网关额度耗尽不当作瞬时抖动

- **GIVEN** 模型网关返回 429「当日额度已用完」
- **WHEN** 消费者处理该失败
- **THEN** 任务直接转 `dead` 并保留该错误，不进行短退避重试，避免继续消耗额度

### Requirement: A6 跨进程索引可见性与最终一致性

任务完成前，材料 SHALL NOT 出现在检索结果里；任务 `succeeded` 后，Web 进程的按班词面索引缓存 SHALL 依据数据库中的切片版本失效并重建，使新切片能被词面路与向量路共同命中。

#### Scenario: A6.1 未完成任务不被检索到

- **GIVEN** 一份材料已提交，任务处于 `pending` 或 `running`
- **WHEN** 本班学生检索该材料的独有词
- **THEN** 结果为空或固定文案「资料中未找到相关内容」，不返回该材料的片段

#### Scenario: A6.2 worker 写库后 Web 缓存失效

- **GIVEN** Web 进程已缓存本班词面索引，且 worker 是另一个进程
- **WHEN** worker 完成任务并写入新切片，随后学生检索新材料的独有词
- **THEN** Web 进程检测到切片版本变化并重建索引，检索能命中新切片

### Requirement: A7 班级隔离与可追溯

任务 SHALL 带 `class_id`；worker 读取材料 SHALL 带班级条件；状态查询 SHALL 按会话班级过滤并统一 404。上传审计 SHALL 记录 `task_id`，任务行 SHALL 记录 `request_id`，任务终态 SHALL 写一条可追溯的审计记录。

#### Scenario: A7.1 跨班任务不可见

- **GIVEN** 班级 B 的任务标识已知
- **WHEN** 班级 A 的用户查询该标识，或 A 班教师对 B 班材料发起重索引
- **THEN** 分别返回 404，且不产生任何针对 B 班材料的任务或索引动作

#### Scenario: A7.2 一次索引可追溯

- **GIVEN** 教师上传材料时带了 `X-Request-ID`
- **WHEN** 任务到达终态
- **THEN** 任务行的 `request_id` 与该请求一致，上传审计含 `task_id`，终态审计含任务标识与结果

### Requirement: A8 三条验收判据可复现

提交响应时间的 P95 SHALL 显著小于一次索引执行耗时，且响应不含结果；停止消费者后提交的任务 SHALL 在消费者恢复后自动执行而不丢；重复提交与重复投递 SHALL 只出一份结果。三条判据 SHALL 各有可复跑的命令与归档记录。

#### Scenario: A8.1 判据一 请求不再等待执行

- **GIVEN** 消费者可正常处理任务，且能观测一次索引的起止时间
- **WHEN** 连续提交 20 次不同材料并用 `curl -w '%{time_total}'` 记录每次提交耗时
- **THEN** 提交耗时 P95 显著小于一次索引执行耗时
- **AND** 20 次响应体都不含索引结果

#### Scenario: A8.2 判据二 任务不因进程终止而丢失

- **GIVEN** 编排文件中消费者是独立服务
- **WHEN** `docker compose stop worker` 后提交一次，再 `docker compose start worker`
- **THEN** 提交返回 202 且状态停留在 `pending`；恢复后无需重新提交即轮询到 `succeeded`

#### Scenario: A8.3 判据三 重复不产生重复结果

- **GIVEN** 同一份提交可重复发送，且可在消费者确认前终止消费者进程
- **WHEN** 连续重复提交，并在 `running` 期间终止消费者再重启
- **THEN** 该提交在 `chunks` 中每个 `seq` 只有一行、向量点数不翻倍、`attempts` 增加而结果不变

### Requirement: A9 未索引材料可一键补齐

教师端材料页 SHALL 提供批量入口，对本班所有 `index_status != ready` 的材料一次触发重索引。入口 SHALL 显示待处理数量，逐批提交并等待任务终态，展示进度与成功/失败计数；遇网关限流 SHALL 停止后续批次并提示已停止，完成后 SHALL 刷新列表。批量入口 SHALL 复用单篇重索引接口，不引入新的写接口。

#### Scenario: A9.1 批量补齐未就绪材料

- **GIVEN** 本班有 3 份 `failed` 材料且消费者正常
- **WHEN** 教师点击「一键索引未索引（3）」
- **THEN** 仅为这 3 份各提交一次重索引任务并等待终态
- **AND** 界面显示进度与成功/失败计数，完成后列表刷新为 `ready`

#### Scenario: A9.2 限流时不继续提交

- **GIVEN** 批量执行中网关返回 429 或额度耗尽
- **WHEN** 当前批次结束
- **THEN** 不再提交后续批次，界面提示已停止及已成功数量

## 能力与场景对照

| 能力 | 场景 |
| --- | --- |
| `async-indexing` | A1.1–A1.3、A2.1–A2.3、A3.1–A3.3、A4.1–A4.3、A5.1–A5.4、A6.1–A6.2、A7.1–A7.2、A8.1–A8.3、A9.1–A9.2 |
| `auth-upload` | R1.1–R1.7、R2.1–R2.3、R3.1–R3.8、R4.1–R4.2 |
