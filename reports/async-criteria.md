# 第 8 课异步三判据复现记录

- 时间：2026-11-17T14:41:30.852Z
- 服务：http://127.0.0.1:8080；compose：-f deploy/docker-compose.yml
- 参数：--criterion 2 --count 20

## 判据二 · 停消费者提交、恢复后自动执行不丢

$ docker compose -f deploy/docker-compose.yml stop worker
```
Container deploy-worker-1 Stopping 
 Container deploy-worker-1 Stopped
```
$ POST /api/materials → 202 {"task_id":"8e345bded861d1d422c2e5d78eb3925c","material_id":385,"status":"pending","status_url":"/api/tasks/8e345bded861d1d422c2e5d78eb3925c"}
- 消费者停止 2 秒后查询：200 {"task_id":"8e345bded861d1d422c2e5d78eb3925c","material_id":385,"kind":"upload","status":"pending","attempts":0,"request_id":"b711f475019bf5ea6d46e4d38a2efbc9","created_at":"2026-11-17T14:41:31.273Z"}
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
$ docker compose -f deploy/docker-compose.yml start worker
```
Container deploy-mysql-1 Waiting 
 Container deploy-mysql-1 Healthy 
 Container deploy-backend-1 Waiting 
 Container deploy-backend-1 Healthy
```
$ docker compose exec -T mysql mysql ... -e "DELETE FROM materials WHERE id IN (385)"

## 汇总

- 判据二：PASS
