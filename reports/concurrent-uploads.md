# 第 8 课异步三判据复现记录

- 时间：2026-11-20T03:26:08.823Z
- 服务：http://127.0.0.1:8081；compose：-f deploy/docker-compose.yml
- 参数：--criterion concurrent --count 20
- 请求 1：HTTP 202；material_id=18；task_id=05d137dee47f6753f1f0a2b6ea8511e8
- 请求 2：HTTP 202；material_id=18；task_id=05d137dee47f6753f1f0a2b6ea8511e8
- 客户端两请求同时在途：5.04 ms
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(DISTINCT m.id), COUNT(t.id) FROM materials m LEFT JOIN index_tasks t ON t.material_id=m.id AND t.class_id=m.class_id AND t.kind='upload' WHERE m.class_id=1 AND m.title='并发上传-bc92698d-c0f7-4a68-b92f-57e4142cfe04'"
```
1	1
```
$ docker compose exec -T mysql mysql ... -e "SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=18 AND class_id=1"
```
1	1
```
$ docker compose exec -T backend sh -c "wget ... /points/count (material_id=18)"
```
{"result":{"count":1},"status":"ok","time":0.000164625}
```
- 材料/上传任务=1	1；切片/唯一序号=1	1；向量点=1
**结论：PASS** — 并发上传：两次 202、相同标识，材料/上传任务/切片/向量点各一份

$ docker compose exec -T mysql mysql ... -e "DELETE FROM materials WHERE id IN (18)"

## 汇总

- 并发上传：PASS
