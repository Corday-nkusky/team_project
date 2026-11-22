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
**复跑**：五服务已启动，执行 `node scripts/async-criteria.mjs --criterion 2` 真机 **PASS**（2026-11-17 晚）：停 worker 后提交返回 `202` + `task_id`，2 秒后查询仍为 `pending`、`attempts=0`；恢复 worker 后终态 `succeeded`、`attempts=1`，无"改了一半"。原始输出 `reports/criterion-2.log`。

## 2026-11-19 操作系统讲义导入
脚本只通过现有 POST /api/materials 上传，沿用材料与任务同事务契约，无直接数据库写入。六份资料不是一个批量事务，中途失败保留已成功项，重跑按同标题同正文跳过；测试覆盖索引失败非零退出。

## A1.2 / A1.4 任务写入失败时回滚与重试

自动化用例位于 `backend/internal/store/upload_rollback_test.go`。测试通过上传使用的存储入口提交材料，在独立测试库中用具名 CHECK 拒绝任务写入，并校验 MySQL 3819 及约束名。从另一条连接核对材料、任务均为 0；解除约束，以相同参数重试，核对两表各有一行，pending 上传任务指向返回的材料。

复跑需要 Go 和专用 MySQL 8.4 实例，测试账号 root 需有创建和删除临时库的权限。设置 `MYSQL_TEST_ADDR` 为该实例的 host:port，有口令时通过环境变量 `MYSQL_TEST_PASSWORD` 提供。测试在结束时清理随机命名的库。

```sh
cd backend
go test -tags=mysqlintegration ./internal/store -run '^TestUploadRollbackAndRetry$' -count=2 -v
```

2026-11-19 本地真实 MySQL 连跑两次通过。另在一次性容器中将材料写入移出事务，测试因 `materials=1 tasks=0, want 0 each` 失败（退出 1）。PR 工作流的 check 作业用专用 MySQL 服务执行该用例。

## 2026-11-21 模型网关验收与索引隔离

集合按嵌入空间隔离，无数据库迁移。切片写入、当前集合内向量重建和材料 ready 沿用原有任务流程。维度检查先于删除和写入，迁移期间保留旧空间向量，查询只用当前空间及本班 ready 切片。

同日范围调整撤回集合拆分，保留原集合、维度检查和索引事务边界。沿用现有嵌入配置升级时，存量向量可直接使用。
