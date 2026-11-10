# Proposal

## Why

课程站第 8 课「异步任务与消息队列」把同步执行列为设计错误：执行时间接近链路最短超时、请求占用处理单元、客户端断开即丢结果、进程重启无记录、算力无法独立伸缩、峰值无缓冲。本仓正踩在前三条上：`POST /api/materials` 与 `POST /api/materials/{id}/reindex` 都在请求内同步跑完 `qa.Index`，其中包含一次最长 120 秒的 embedding 调用，且全程使用 `r.Context()`——客户端断开或超时会直接取消建索引；任务没有持久化标识，无法查询、重试或在重启后恢复。

课件 `apply.html` 要求提交接口返回 `202 + task_id + status_url`、消费者可独立启停、三条判据可复现；`redesign.html` 要求业务写入与任务记录同事务（事务性发件箱）；`retry.html` 要求失败分类、次数上限、指数退避加抖动、死信，以及由业务含义派生的幂等键落在唯一约束上。现有 `auth-upload` 规约 R3.1/R3.5/R3.6 白纸黑字要求「201 + 立即 ready」，与 202 直接冲突，必须先走 spec delta。

异步不改变做完这件事要多久，只改变谁在等：把一次可能被 120 秒外部调用拖死、且随连接生死的请求，变成一条库里可查、可重试、可去重、按班级隔离的记录。问答的 SSE 流式链路、登录/刷新/登出/权限判定与探活保持同步——调用方缺少结果就无法继续，改成轮询是负收益。

## What Changes

- 新增 `index_tasks` 表与幂等迁移：`materials` 补 `content_hash` / `revision`，`chunks` 补 `(material_id, seq)` 唯一约束；迁移可重复执行，旧行不破坏。
- `store` 新增事务性发件箱：上传把「材料 + 任务」写进同一事务；重索引按幂等键入队或重放终态任务；`SKIP LOCKED` 领取、租约回收、状态流转、完成与失败落库都在 `store` 内。
- 上传与重索引关闭请求内 `qa.Index` 调用，改返回 `202` 与任务标识、`status_url`；响应体不含切片数、不含 `ready`。
- 新增只读 `GET /api/tasks/{task_id}`，查询带 `class_id`，跨班 404，绝不触发执行。
- 新增 `internal/indexer` 与 `cmd/worker`：独立消费者进程，用 MySQL 8.4 `FOR UPDATE SKIP LOCKED` 竞争消费、租约超时回收实现至少一次投递；失败分 transient / permanent / 限流三类，指数退避加抖动，超限转死信并保留原因。
- 幂等三落点：入口幂等键唯一约束、`chunks` 唯一约束、Qdrant 点 ID 由 `material_id + seq + 内容` 确定性派生。
- Web 进程的按班词面索引缓存改为按 `chunks` 版本失效，解决 worker 写库后 Web 缓存不更新的跨进程可见性问题。
- 编排文件新增可独立 stop/start 的 `worker` 服务，复用同一后端镜像；调优参数全部外置且带显式默认值。
- 前端上传/重索引适配 202 并轮询状态；`scripts/reindex-all.mjs`、Go e2e、浏览器 e2e 与 README 同步新契约；主规约 R3 与新能力 `async-indexing` 同步。

## Capabilities

### Modified Capabilities

- `auth-upload`：R3「教学材料上传与索引」由「请求内建完索引并返回 201 + ready」改为「同事务写入任务并返回 202 + 任务标识，轮询至 succeeded 后本班可检索」。R3.1、R3.3、R3.5、R3.6 场景的承诺随之修改；R3.2、R3.4、R3.7、R3.8 标题与承诺保持不变，按规约要求一并搬入 delta。

### Added Capabilities

- `async-indexing`：任务表与事务性发件箱、202 契约、四态状态机加死信、只读状态接口、独立消费者与至少一次投递、三处幂等、失败分类与退避、跨进程索引可见性、班级隔离、三条验收判据。

## Impact

- 接口：`POST /api/materials` 与 `POST /api/materials/{id}/reindex` 由 201/200 改 202；新增 `GET /api/tasks/{task_id}`。旧客户端若按 201 判定成功会失败，属本次契约变更，需在 README 与前端同步。
- 部署：Compose 新增 `worker` 服务；判据二可用 `docker compose stop worker` / `start worker` 原样复现。不新增 RabbitMQ / Redis 容器，不新增第三方 Go 依赖。
- 数据：新增一张表、两列、两条唯一约束；旧材料的 `content_hash` 为 NULL，唯一索引允许多个 NULL；已存在的重复 `(material_id, seq)` 在加约束前按保留最小 `id` 清理。
- 行为：未完成任务期间材料 `index_status` 保持 `pending`，`ListChunksByClass` 只取 `ready`，检索不会出现「已提交但尚未生效」的半截结果；零密钥降级、班级隔离、引用契约、SSE 问答均不变。
- 验证：离线门禁（分层 / 密钥 / gofmt / build / vet / test / OpenSpec strict）必须全绿；三条判据首轮以脚本与 Go e2e 复现，实测写进 `tasks.md` 与 `reports/`。需要真实服务与模型额度的项目如实标注未验证。
