# 高风险三查 · 并发

> 四件事齐全：用什么手段查 / 查了什么 / 判据 / 结果。最终报告「高风险三查 · 并发」直接素材。
> 数据来自 2026-11-10 真机复现（`reports/async-criteria.md` 判据三，五服务运行时），命令可当场重放。

## 四件事

- **用什么手段查**：真实 HTTP 并发提交 + 直连 MySQL 断言唯一约束；脚本 `scripts/async-criteria.mjs`（`--criterion 3`）；门禁常驻用例 `go test ./internal/store ./internal/httpapi`。
- **查了什么**：同一份材料内容连续提交两次（模拟"两个请求同时到达"与"重复投递"），检查是否产生两条材料 / 两份切片 / 两个向量点。
- **判据**：同一内容只出一份结果——`task_id` 相同、`material_id` 相同、`chunks` 只有 1 行（`COUNT(*)=COUNT(DISTINCT seq)=1`）、Qdrant 点数 = 1。唯一性必须由数据库约束保证，不靠"先查一遍"。
- **结果**：见下。

## 复现命令与原始输出

```
$ 连续 POST /api/materials 同一内容两次 → 202 / 202
- task_id：1e41770d020569b5a4d19c27a7e32932 / 1e41770d020569b5a4d19c27a7e32932
- material_id：280 / 280

$ docker compose exec -T mysql mysql ... -e \
    "SELECT COUNT(*), COUNT(DISTINCT seq) FROM chunks WHERE material_id=280"
1    1

$ docker compose exec -T backend sh -c "wget ... /points/count (material_id=280)"
{"result":{"count":1},"status":"ok","time":0.001828782}
```

（来源：`reports/async-criteria.md:47-57`，2026-11-10T13:40:49Z 真机运行记录。）

## 它想保证什么 / 靠什么保证 / 什么时候失效 / 挡得住的写法

1. **想保证**：同一份讲义只索引一次，重复提交不产生重复材料、切片或向量点。
2. **靠什么保证**：`ContentHash(title, content)` 派生确定性幂等键；`materials` 上 `uk_material_hash(class_id, content_hash)`、`index_tasks` 上 `uk_idem(idempotency_key)`、`chunks` 上 `uk_chunk_seq(material_id, seq)`——全部是数据库唯一约束。
3. **什么时候失效**：若用"先 `SELECT` 有没有，没有才 `INSERT`"的写法，两个请求可能同时查完都发现没有，于是都插入——即课程举例的经典并发 bug。
4. **挡得住的写法**：把唯一性交给数据库约束，插入遇到唯一键冲突就按既定语义返回既有记录（本仓 `UpsertMaterial` / `EnqueueTask` 的做法）。

## 结论

**判据满足（绿）**：两次提交得到同一 `task_id` / `material_id`，chunks 与向量点都只有 1 份，且由唯一约束兜底。用例常驻门禁（`go test ./internal/store` 覆盖幂等键）。
**复跑**：五服务已启动，执行 `node scripts/async-criteria.mjs --criterion 3 --count 20` 真机 **PASS**（2026-11-17 晚）：同一内容两次提交复用同一 `task_id` / `material_id`（382），切片每 seq 一行、向量点 1 且不翻倍；捕获 running 后 kill worker 并注入租约过期，恢复后终态 `succeeded`、`attempts=2`。原始输出 `reports/criterion-3.log`。

## 2026-11-19 操作系统讲义导入
顺序导入六份正文；客户端标题查询仅用于友好去重，最终并发唯一性继续由服务端 uk_material_hash 与任务幂等键约束保证，不把先查后写当作并发保证。不更改 worker 或唯一键。
