# Design

## D1 选型：MySQL 任务表 + 独立 worker，不引消息代理容器

依据来自课程站原文，不是省事：`queue.html`「本课不要求特定实现，判定标准是三条验收判据能否复现」；`think.html` 第 3 组第 3 题明确写「DB 任务表加定时扫描，三条判据都能达到，但需要自行实现行级抢占、超时回收、退避与死信；取舍依据是这些机制由谁保证」。因此本仓把队列机制落在 `index_tasks` 表与 `internal/indexer` 里，由 MySQL 8.4 的 `FOR UPDATE SKIP LOCKED` 提供行级抢占（官方手册称其适用于 queue-like table），由 `lease_until` 实现可见性超时回收。

`apply.html` 第一步要求「编排文件中新增队列服务，配置外置」、判据二要求 `docker compose stop <消费者服务名>`。本仓以**可独立启停的 `worker` 服务**满足消费者独立这一条，队列持久化由 MySQL 承担；worker 与 Web 使用同一镜像、同一份配置、不同入口，可单独停止、单独扩容。不引入 RabbitMQ / Redis：本仓是单班教学材料（百篇量级），加一个中间件会带来新的容器、连接配置与故障面，却换不回等价的判据收益；跨机水平扩展的升级路径是换投递器，不改变任务表契约。

## D2 任务记录与事务性发件箱

`index_tasks` 字段：`task_id`（对外 CHAR(32)）、`class_id`、`material_id`、`kind`、`status`、`attempts`、`idempotency_key`、`error`、`request_id`、`run_at`、`lease_until`、`started_at`、`finished_at`、`created_at`。唯一约束 `uk_task_id`、`uk_idem`；索引 `idx_task_claim(status, run_at)` 供扫描，`idx_task_class(class_id, task_id)` 供班级过滤查询。

`CreateMaterialWithTask` 在一个 InnoDB 事务里写 `materials` 与 `index_tasks`；提交成功必然有任务，回滚则两者都不留。这正是 `redesign.html` 要求的前一种写法（先写待办，再由独立消费者扫描 pending），不是「提交后直接投递」。投递延迟等于 worker 轮询间隔（默认 500ms）。

`materials` 新增 `content_hash CHAR(64) NULL`（旧行 NULL，MySQL 唯一索引允许多个 NULL）与 `revision INT NOT NULL DEFAULT 0`（每次索引成功后 +1，供内容未变时的重索引区分版本）。

## D3 状态机、重试与失败分类

```
pending --(SKIP LOCKED 领取, attempts+1)--> running --(成功)--> succeeded
                                              +--(transient, attempts < 上限)--> pending(run_at = NOW()+退避)
                                              +--(transient, attempts >= 上限)--> dead
                                              +--(permanent / 网关额度 429)--> failed / dead（保留 error）
```

- 成功：`succeeded` 与 `materials.index_status='ready'`、空 `index_error`、`revision+1` 在同一事务完成。
- `running` 且 `lease_until < NOW()` 的任务在 worker 启动时回收为 `pending`，`attempts` 不回退——这是至少一次投递，判据三靠消费端幂等吸收，判据二靠任务仍在表里。
- transient：`qa.Index` 的 DB / Qdrant / 网络 / 5xx / 超时错误。退避 `base * 2^(attempts-1)`，叠加 `±20%` 抖动，上限 `WORKER_BACKOFF_MAX_MS`。
- permanent：`qa.ErrPermanent`（切片为空、材料不存在等同样输入必然同结果的错误）直接置 `failed`，不消耗重试额度。
- 限流：`llm.ErrRateLimited`（课程网关按天限额，实测 429）直接转 `dead` 并保留原文，短退避重试只会继续烧额度；人工确认额度恢复后通过重索引接口重放同一任务。
- 所有终态都把 `materials.index_status` 置 `failed` 并写 `index_error`；`dead` 即死信，保留任务行、`error`、`attempts` 与消息关联，供 `reports/` 统计。

## D4 幂等的三个落点

1. **入口**：`idempotency_key` 由业务含义派生，不是随机值——上传 `upload:<class_id>:<sha256(title+\0+content)>`，重索引 `reindex:<material_id>:<sha256(content)>:r<revision>`。`uk_idem` 唯一约束配合 `INSERT ... ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)` 取回已有任务；重复提交不产生第二条任务，也不产生第二份材料（`uk_material_hash` 复用同一材料行）。重索引命中 `failed/dead` 的旧任务时重放同一行（`attempts` 归零、`run_at=NOW()`），而不是新建任务。
2. **写切片结果**：`chunks` 加 `UNIQUE KEY uk_chunk_seq(material_id, seq)`，`ReplaceChunks` 仍是一个事务内 `DELETE` + 批量 `INSERT`；并发重复执行被数据库约束吸收，不会留下两套切片。
3. **写向量库**：Qdrant 点 ID 由 `fnv64(material_id + seq + 内容摘要)` 确定性派生并压到 2^53 以内，去掉对自增 `chunk.ID` 的依赖。Qdrant 的 upsert 语义是同 ID 覆盖，重复消费写出的点集合完全一致；`DeleteByMaterial` 仍用于清理 seq 变少的旧点，但不再是幂等的依靠。payload 里的 `chunk_id` 仍是本轮 MySQL 切片主键，供词面索引校验。

## D5 跨进程索引可见性

Web 进程的 `qa.Service` 按班缓存 BM25 倒排索引，原先只在**本进程**调用 `Index` 时标脏。worker 独立进程写库后，Web 缓存不会自动失效，新材料会被 `idx.Doc(id)` 过滤掉，向量命中也进不了候选。

解法：`store.ChunkVersion(ctx, classID)` 返回 `SELECT COALESCE(MAX(id),0) FROM chunks WHERE class_id=?`，`classIndex` 在缓存命中时先比对版本，变化即重建（仍在 `buildMu` 下二次检查）。这是一次走 `idx_chunk_class` 的轻量查询，换来的是 worker 与 Web 解耦后的正确性；`s.invalidate` 保留给同进程路径。

## D6 班级隔离与只读接口

- `index_tasks` 带 `class_id`，worker 取材料走 `GetMaterial(ctx, materialID, classID)`，绝不使用不带班级条件的路径。
- `GET /api/tasks/{task_id}` 只需登录，`WHERE task_id=? AND class_id=?`；跨班与不存在统一 404，不泄漏存在性，也不触发任何执行（`think.html`：查询接口有副作用会让刷新、重试、探活都触发执行）。
- 上传与重索引仍只限教师；`request_id` 串起 HTTP 请求与任务，`upload` 审计记 `task_id`，worker 终态写一条 `index_task` 审计，便于「谁在何时触发了哪次索引」。

## D7 分层与配置

新增 `internal/indexer`（层 2，编排 worker 生命周期）与 `cmd/worker`（层 3，装配入口），在 `scripts/check-layering.mjs` 的 LAYERS 登记；`httpapi` 新增 `tasks.go` 并加入 store 白名单（单表读写）。`indexer` 依赖 `qa`(1)、`store`(0)、`config`(0)，不反向依赖 `httpapi`。

调优项全部外置且带显式默认值：`WORKER_POLL_MS=500`、`WORKER_LEASE_SECONDS=300`、`WORKER_TASK_TIMEOUT_SECONDS=600`、`WORKER_MAX_ATTEMPTS=5`、`WORKER_BACKOFF_MS=1000`、`WORKER_BACKOFF_MAX_MS=60000`、`WORKER_BATCH=4`。显式设置但无法解析时按 R8 在启动前报错，日志只报键名。

## D8 观测与证据

任务行自带 `status` / `attempts` / `started_at` / `finished_at` / `error`，直接产出队列深度（`status='pending'` 计数）、消费耗时（`finished_at - started_at`）、重试次数、死信数（`status='dead'`）。`scripts/async-criteria.mjs` 复现三条判据并输出命令与结论，证据按课程要求归到仓库根 `reports/`。

## D9 非目标

不改问答 SSE、登录 / 刷新 / 登出 / 权限判定 / 探活；不引入 RabbitMQ / Redis / 新的第三方 Go 库；不新建评测体系；不用 SSE 推送任务状态（本课采用轮询）；不实现任务删除 / 管理界面；不做跨机多 worker 的容量规划（单 worker 起步，`SKIP LOCKED` 已为多实例留出正确路径）。
