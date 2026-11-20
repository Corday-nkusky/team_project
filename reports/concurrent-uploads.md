# 第 8 课并发上传验收记录

- 时间：2026-11-20T07:20:37.921Z
- 服务：http://127.0.0.1:8080；compose：-f deploy/docker-compose.yml
- 参数：--criterion concurrent
- 请求 1：HTTP 202；material_id=389；task_id=b009eabfd3d8400ec527e93406b66f73
- 请求 2：HTTP 202；material_id=389；task_id=b009eabfd3d8400ec527e93406b66f73
- 服务端请求标识：ef7086bad0c77c651ee22d31b19cfd91 / 56a479f0b5346e3ed474c34a37686651（两条不同，说明服务端收到两次独立请求）
$ docker compose logs --tail 400 backend | grep <两条 request id>
```
backend-1  | {"time":"2026-11-20T07:20:38.026760138Z","level":"INFO","msg":"http_request","request_id":"ef7086bad0c77c651ee22d31b19cfd91","method":"POST","path":"/api/materials","status":202,"duration_ms":8}
backend-1  | {"time":"2026-11-20T07:20:38.027966449Z","level":"INFO","msg":"http_request","request_id":"56a479f0b5346e3ed474c34a37686651","method":"POST","path":"/api/materials","status":202,"duration_ms":9}
```
- 客户端两请求窗口重叠：12.91 ms（客户端观测值，非服务端并发直证）
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(DISTINCT m.id), COUNT(t.id) FROM materials m LEFT JOIN index_tasks t ON t.material_id=m.id AND t.class_id=m.class_id AND t.kind='upload' WHERE m.class_id=1 AND m.title='并发上传-f4c59092-d99b-4f35-8c0a-cd2855acacee'"
```
1	1
```
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=389 AND class_id=1"
```
1	1
```
$ docker compose exec -T backend sh -c "wget ... /points/count (material_id=389)"
```
{"result":{"count":1},"status":"ok","time":0.00329263}
```
- 材料/上传任务=1	1；切片/唯一序号=1	1；向量点=1
**结论：PASS** — 并发上传：两次 202、相同标识，材料/上传任务/切片/向量点各一份

$ docker compose exec -T mysql mysql ... -e "DELETE FROM materials WHERE id IN (389)"

## 汇总

- 并发上传：PASS
