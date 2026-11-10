# 第 8 课异步三判据复现记录

- 时间：2026-11-10T13:40:49.451Z
- 服务：http://127.0.0.1:8080；compose：-f deploy/docker-compose.yml
- 参数：--criterion all --count 20

## 判据一 · 提交 P95 ≪ 执行耗时，且响应不含结果

- 本批 20 个任务到达终态：20
$ 连续 POST /api/materials 20 次（curl -w '%{time_total}' 等价：脚本内 performance.now()）
- 每次提交耗时（ms，升序）：7.7, 7.9, 8.1, 8.2, 8.2, 8.4, 8.5, 8.5, 8.8, 8.9, 9.3, 9.4, 9.5, 9.7, 11.1, 11.7, 12.9, 14.2, 16.8, 17.6
- P95 = **17.6 ms**；首个任务状态 = succeeded
- 一次索引执行耗时（finished_at − started_at）= 216 ms
- 202 且含 task_id / status_url：true；响应不含索引结果：true
**结论：PASS** — 判据一：P95 17.6ms vs 执行 216ms；不含结果=true。


## 判据二 · 停消费者提交、恢复后自动执行不丢

$ docker compose -f deploy/docker-compose.yml stop worker
```
Container deploy-worker-1 Stopping 
 Container deploy-worker-1 Stopped
```
$ POST /api/materials → 202 {"task_id":"c8d6b70084b93c5631db806a8e4e621e","material_id":279,"status":"pending","status_url":"/api/tasks/c8d6b70084b93c5631db806a8e4e621e"}
- 消费者停止 2 秒后查询：200 {"task_id":"c8d6b70084b93c5631db806a8e4e621e","material_id":279,"kind":"upload","status":"pending","attempts":0,"request_id":"bb6e9960614349013965ee77b23aff45","created_at":"2026-11-10T13:40:54.463Z"}
$ docker compose -f deploy/docker-compose.yml start worker
```
Container deploy-mysql-1 Waiting 
 Container deploy-mysql-1 Healthy 
 Container deploy-backend-1 Waiting 
 Container deploy-backend-1 Healthy 
 Container deploy-worker-1 Starting 
 Container deploy-worker-1 Started
```
- 恢复消费者后终态：succeeded attempts=1
**结论：PASS** — 判据二：停止期间状态=pending，恢复后=succeeded

$ docker compose -f deploy/docker-compose.yml start worker
```
Container deploy-mysql-1 Waiting 
 Container deploy-mysql-1 Healthy 
 Container deploy-backend-1 Waiting 
 Container deploy-backend-1 Healthy
```

## 判据三 · 重复提交 / 重复投递只出一份结果

$ 连续 POST 同一份内容两次 → 202 / 202
- task_id：1e41770d020569b5a4d19c27a7e32932 / 1e41770d020569b5a4d19c27a7e32932；material_id：280 / 280
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=280"
```
1	1
```
$ docker compose exec -T backend sh -c "wget ... /points/count (material_id=280)"
```
{"result":{"count":1},"status":"ok","time":0.001828782}
```
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=280"
```
1	1
```
$ docker compose exec -T backend sh -c "wget ... /points/count (material_id=280)"
```
{"result":{"count":1},"status":"ok","time":0.001718777}
```
- 首次完成：succeeded；切片 1 行 / 1 个 seq；向量点 1
- 再次重索引：succeeded；切片 1 行 / 1 个 seq；向量点 1
$ docker compose -f deploy/docker-compose.yml stop worker
```
Container deploy-worker-1 Stopping 
 Container deploy-worker-1 Stopped
```
$ docker compose -f deploy/docker-compose.yml start worker
```
Container deploy-mysql-1 Waiting 
 Container deploy-mysql-1 Healthy 
 Container deploy-backend-1 Waiting 
 Container deploy-backend-1 Healthy 
 Container deploy-worker-1 Starting 
 Container deploy-worker-1 Started
```
$ docker compose -f deploy/docker-compose.yml kill worker
```
Container deploy-worker-1 Killing 
 Container deploy-worker-1 Killed
```
$ docker compose exec -T mysql mysql ... -e "UPDATE index_tasks SET lease_until=DATE_SUB(NOW(3), INTERVAL 1 SECOND) WHERE task_id='58d9f989d3d4d09dd4095c2d23220844' AND status='running'"
$ docker compose -f deploy/docker-compose.yml start worker
```
Container deploy-mysql-1 Waiting 
 Container deploy-mysql-1 Healthy 
 Container deploy-backend-1 Waiting 
 Container deploy-backend-1 Healthy 
 Container deploy-worker-1 Starting 
 Container deploy-worker-1 Started
```
- 捕获 running 后 kill worker，注入租约过期再启动；终态 succeeded，attempts=2
**结论：PASS** — 判据三：入口幂等=true；切片每 seq 一行且不增长=true；向量点不翻倍=true；重复投递 attempts≥2=true。

$ docker compose -f deploy/docker-compose.yml start worker
```
Container deploy-mysql-1 Waiting 
 Container deploy-mysql-1 Healthy 
 Container deploy-backend-1 Waiting 
 Container deploy-backend-1 Healthy
```
$ docker compose exec -T mysql mysql ... -e "DELETE FROM materials WHERE id IN (259,260,261,262,263,264,265,266,267,268,269,270,271,272,273,274,275,276,277,278,279,280,282)"

## 汇总

- 判据一：PASS
- 判据二：PASS
- 判据三：PASS
