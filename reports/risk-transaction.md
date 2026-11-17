# 高风险三查 · 事务

> 四件事齐全：用什么手段查 / 查了什么 / 判据 / 结果。最终报告「高风险三查 · 事务」直接素材。
> 数据来自 2026-11-10 真机复现（`reports/async-criteria.md` 判据二）。

## 四件事

- **用什么手段查**：停消费者 → 提交 → 查状态停在 pending → 恢复消费者 → 查终态；直连 MySQL 核对任务记录与材料记录。脚本 `scripts/async-criteria.mjs`（`--criterion 2`）。
- **查了什么**：业务写入（材料 + 任务记录）是否在同一事务；消费者停止期间提交是否"改了一半"；恢复后是否恰好执行一次、无孤儿任务。
- **判据**：停消费者期间状态为 `pending` 且 `attempts=0`（材料已写入、任务已入队但没执行）；恢复后恰好一次执行到 `succeeded`、`attempts=1`；不出现"材料有但任务没有"或"任务 succeed 但切片缺失"。
- **结果**：见下。

## 复现命令与原始输出

```
$ docker compose -f deploy/docker-compose.yml stop worker
 Container deploy-worker-1 Stopped

$ POST /api/materials → 202
{"task_id":"c8d6b70084b93c5631db806a8e4e621e","material_id":279,
 "status":"pending","status_url":"/api/tasks/c8d6b70084b93c5631db806a8e4e621e"}

$ 停止 2 秒后查询任务状态 → 200
{"task_id":"c8d6b70084b93c5631db806a8e4e621e","material_id":279,
 "status":"pending","attempts":0,...,"created_at":"2026-11-10T13:40:54.463Z"}

$ docker compose -f deploy/docker-compose.yml start worker
 Container deploy-worker-1 Started

$ 恢复后查询终态
status=succeeded  attempts=1
```

（来源：`reports/async-criteria.md:18-37`）

## 它想保证什么 / 靠什么保证 / 什么时候失效 / 挡得住的写法

1. **想保证**："提交材料"与"生成索引任务"要么都成功、要么都不生效，不出现改了一半的库。
2. **靠什么保证**：`backend/internal/store/tasks.go` 的事务性发件箱——材料写入与 `index_tasks` 记录在**同一个 SQL 事务**里提交；worker 用 `FOR UPDATE SKIP LOCKED` 抢占、`lease_until` 处理超时回收。
3. **什么时候失效**：若先写材料、`commit` 后再单独插入任务，中间的崩溃会留下"有材料无任务"的孤儿，材料永远不会被索引。
4. **挡得住的写法**：把业务写入与发件箱记录放进同一事务（本仓 `EnqueueIndexTask` 的做法），停止消费者只影响任务执行、不影响业务写入的一致性。

## 结论

**判据满足（绿）**：停消费者期间 `pending` / `attempts=0`，恢复后 `succeeded` / `attempts=1`，无"改了一半"现象；事务性发件箱实现见 `store/tasks.go`。
**复跑**：五服务启动后执行 `node scripts/async-criteria.mjs --criterion 2`；本轮 Docker daemon 未运行，引用 2026-11-10 原始记录。
