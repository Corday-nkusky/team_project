# 第 8 课异步三判据复现记录

- 时间：2026-11-20T22:57:50.097Z
- 服务：http://127.0.0.1:8080；compose：-f deploy/docker-compose.yml
- 参数：--criterion all --count 20

## 判据一 · 提交 P95 ≪ 执行耗时，且响应不含结果

- 本批 20 个任务到达终态：20
$ 连续 POST /api/materials 20 次（curl -w '%{time_total}' 等价：脚本内 performance.now()）
- 每次提交耗时（ms，升序）：8.0, 8.2, 8.2, 8.3, 8.3, 8.6, 8.6, 8.8, 8.8, 8.9, 9.1, 9.3, 9.4, 9.4, 9.5, 9.9, 10.0, 10.0, 10.1, 13.6
- P95 = **13.6 ms**；首个任务状态 = succeeded
- 一次索引执行耗时（finished_at − started_at）= 10 ms
- 202 且含 task_id / status_url：true；响应不含索引结果：true
**结论：FAIL** — 判据一：P95 13.6ms vs 执行 10ms；不含结果=true。


## 判据二 · 停消费者提交、恢复后自动执行不丢

$ docker compose -f deploy/docker-compose.yml stop worker
```
Container campusclaw-clean-20261121-worker-1 Stopping 
 Container campusclaw-clean-20261121-worker-1 Stopped
```
$ POST /api/materials → 202 {"task_id":"f185ab7ca890c36603bd2fb30a494395","material_id":59,"status":"pending","status_url":"/api/tasks/f185ab7ca890c36603bd2fb30a494395"}
- 消费者停止 2 秒后查询：200 {"task_id":"f185ab7ca890c36603bd2fb30a494395","material_id":59,"kind":"upload","status":"pending","attempts":0,"request_id":"ebe06843935634578ef68afa3bd56942","created_at":"2026-11-20T22:57:52.865Z"}
$ docker compose -f deploy/docker-compose.yml start worker
```
Container campusclaw-clean-20261121-mysql-1 Waiting 
 Container campusclaw-clean-20261121-mysql-1 Healthy 
 Container campusclaw-clean-20261121-backend-1 Waiting 
 Container campusclaw-clean-20261121-backend-1 Healthy 
 Container campusclaw-clean-20261121-worker-1 Starting 
 Container campusclaw-clean-20261121-worker-1 Started
```
- 恢复消费者后终态：succeeded attempts=1
**结论：PASS** — 判据二：停止期间状态=pending，恢复后=succeeded

$ docker compose -f deploy/docker-compose.yml start worker
```
Container campusclaw-clean-20261121-mysql-1 Waiting 
 Container campusclaw-clean-20261121-mysql-1 Healthy 
 Container campusclaw-clean-20261121-backend-1 Waiting 
 Container campusclaw-clean-20261121-backend-1 Healthy
```

## 判据三 · 并发提交 / 重复投递只出一份结果

- 请求 1：HTTP 202；material_id=60；task_id=885005692c44a866d2d8b5173ee1e646
- 请求 2：HTTP 202；material_id=60；task_id=885005692c44a866d2d8b5173ee1e646
- 服务端请求标识：7fcab5235ff459dca2981c3bddf4e183 / b909a4f3252ac2185881fe347d905da1（两条不同，说明服务端收到两次独立请求）
$ docker compose logs --tail 400 backend | grep <两条 request id>
```
backend-1  | {"time":"2026-11-20T22:57:57.3195939Z","level":"INFO","msg":"http_request","request_id":"7fcab5235ff459dca2981c3bddf4e183","method":"POST","path":"/api/materials","status":202,"duration_ms":8}
backend-1  | {"time":"2026-11-20T22:57:57.32091566Z","level":"INFO","msg":"http_request","request_id":"b909a4f3252ac2185881fe347d905da1","method":"POST","path":"/api/materials","status":202,"duration_ms":9}
```
- 客户端两请求窗口重叠：10.70 ms（客户端观测值，非服务端并发直证）
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(DISTINCT m.id), COUNT(t.id) FROM materials m LEFT JOIN index_tasks t ON t.material_id=m.id AND t.class_id=m.class_id AND t.kind='upload' WHERE m.class_id=1 AND m.title='并发上传-100eff91-9233-4c4d-926d-125252647266'"
```
1	1
```
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=60 AND class_id=1"
```
1	1
```
$ docker compose exec -T backend sh -c "wget ... /points/count (material_id=60)"
```
wget: server returned error: HTTP/1.1 404 Not Found
```
- 材料/上传任务=1	1；切片/唯一序号=1	1；向量点=未采集

**脚本异常**：Error: 单切片材料应恰有一行材料、一个上传任务、一条切片和一个向量点
    at concurrentUpload (file:///D:/code/team-project-clean-smoke-20261121/scripts/async-criteria.mjs:276:11)
    at process.processTicksAndRejections (node:internal/process/task_queues:104:5)
    at async criterion3 (file:///D:/code/team-project-clean-smoke-20261121/scripts/async-criteria.mjs:283:22)
    at async file:///D:/code/team-project-clean-smoke-20261121/scripts/async-criteria.mjs:342:41
$ docker compose -f deploy/docker-compose.yml start worker
```
Container campusclaw-clean-20261121-mysql-1 Waiting 
 Container campusclaw-clean-20261121-mysql-1 Healthy 
 Container campusclaw-clean-20261121-backend-1 Waiting 
 Container campusclaw-clean-20261121-backend-1 Healthy
```
$ docker compose exec -T mysql mysql ... -e "DELETE FROM materials WHERE id IN (39,40,41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60)"

## 汇总

- 判据一：FAIL
- 判据二：PASS
- 执行：FAIL
