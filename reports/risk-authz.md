# 高风险三查 · 权限（越权）

> 四件事齐全：用什么手段查 / 查了什么 / 判据 / 结果。最终报告「高风险三查 · 权限」直接素材。
> 复测命令可在演示机器原样再跑（五服务启动后）。

## 四件事

- **用什么手段查**：真实 HTTP 请求（curl / e2e / 单测桩），用种子身份 `student1`（班级 1）、`student2`（班级 2）、`teacher2`（班级 2）。
- **查了什么**：B 班账号读 A 班讲义；A 班账号读 B 班讲义；学生调用教师专属接口；跨班任务查询；令牌篡改与缺失。
- **判据**：跨班读取返回 404（与资源不存在同形，不泄漏存在性）；跨班检索为空；学生写操作 403；无令牌/篡改令牌 401。
- **结果**：见下（e2e 与单测均为真实执行输出）。

## 复测命令与原始输出

```
# 1) A 班学生读 B 班材料详情（期望 404，且响应不含 B 班内容）
GET /api/materials/{B班材料id}  Authorization: Bearer <student1>
→ e2e checkCrossClassReindex: "A 班学生读取 B 班材料详情 → 404 且不含 B 内容"  PASS
  （backend/cmd/e2e/main.go:638-640）

# 2) A 班教师读 B 班材料详情与重索引
GET /api/materials/{B班材料id}        → 404
POST /api/materials/{B班材料id}/reindex → 404/403 且不含 B 班独有密语
  （main.go:633-637）

# 3) 他班教师查询本班任务
GET /api/tasks/{A班task_id}  Authorization: Bearer <teacher2>
→ 404 且不泄漏任务 id（main.go:262）

# 4) 学生写操作
POST /api/materials         → 403 "学生调用上传接口 → 403"（main.go:219）
POST /api/skills            → 403（单测 TestUpsertSkillStudentForbidden）
GET /api/mcp/calls          → 403（单测 TestStudentCannotListMCPCalls）

# 5) 认证负向
无 Authorization            → 401（main.go:190）
篡改一字节的 access token    → 401（main.go:188）
账号不存在 / 口令错误         → 401（main.go:179-180）
```

## 它想保证什么 / 靠什么保证 / 什么时候失效 / 挡得住的写法

1. **想保证**：用户只能读/写本班数据；角色不符一律拒绝；跨班与不存在不可区分。
2. **靠什么保证**：`requireAuth`（JWT 签名 + 过期）+ `requireRole("teacher")`；所有业务 SQL 都带 `class_id` 条件（"查询即隔离"，不是"先查后判"）。
3. **什么时候失效**：若接口只检查"登录了没有"，不检查数据归属，换 URL 里的 id 就能看别人的；若用"先查出来再看 class_id 对不对"，越权数据在内存里已经出现过，且容易漏掉某个出口。
4. **挡得住的写法**：把 `class_id` 作为 WHERE 的一部分（如 `SELECT ... WHERE class_id=? AND id=?`），查不到就返回 404；角色检查在中间件层统一拦截，不在 handler 里散落判断。

## 结论

**判据满足（绿）**：e2e 与单测均实测 401/403/404 负向用例；班级隔离由 SQL 条件保证，不泄漏资源存在性。
**复跑**：五服务已启动，`cd backend && go run ./cmd/e2e` 真机跑出 **81 通过 / 0 失败**（覆盖 R1/R2 权限负向断言），原始输出见 `reports/e2e-run.log`。

## 2026-11-19 操作系统讲义导入
操作系统入口固定目标 class_id=1，登录后校验 teacher 角色与班级，错误身份在首次上传前失败；模拟学生和跨班教师测试均断言上传次数为零。沿用服务端 JWT 与班级隔离，不新增权限入口。

## 2026-11-21 模型网关验收与索引隔离

向量查询继续按请求身份推导的 `class_id` 过滤，并与本班 ready 切片核对。索引删除同时按材料和班级匹配，`TestDeleteByMaterialRequiresClassAndMaterial` 核对请求。新增 SSE 字段仅描述语义路径和已获授权的引用。

同日范围调整恢复原索引写入路径。材料标题先按班级和全局唯一材料 ID 查询，向量清理按该材料 ID 定位，问答继续以班级和 ready 状态限制引用。
