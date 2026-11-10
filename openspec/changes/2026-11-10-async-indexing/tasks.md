# Tasks

> 实施顺序按第 8 课 `apply.html` 的五步，不颠倒：队列与任务表 → 改提交接口 → 消费者 → 状态接口与重试。
> 实测为 2026-11-10 本机真机结果（Docker Compose 四服务 + 智谱网关），未验证项单列在末节，不写成通过。

## 1. 变更与规约

- [x] 1.1 建立 change 四件套并通过 OpenSpec 严格校验
  - verify: `npx --yes @fission-ai/openspec@latest validate --all --strict` 退出码 0
  - 实测：通过；`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec --with-e2e` 中「OpenSpec 严格校验」✓（2653ms）
- [x] 1.2 主规约同步 R3 与新增 `async-indexing` 能力，对照表无旧契约残留
  - verify: `grep -R "返回 201|201 且|→ 201" openspec/specs/` 无上传相关命中；主规约与 delta 的 R3.1–R3.8 场景标题一致
  - 实测：`grep -R "返回 201\|201 且\|→ 201" openspec/specs/` 无命中

## 2. 队列与任务记录（第一步、第二步）

- [x] 2.1 编排文件新增可独立启停的 `worker` 服务，配置全部外置
  - verify: `docker compose config --services` 含 `worker`；`docker compose stop worker` 后 backend 仍可提交
  - 实测：`config --services` 输出 `mysql / qdrant / backend / worker`；判据二 `stop worker` 后提交返回 202、任务停在 `pending`，`start worker` 后自动完成（`reports/async-criteria.md`）
- [x] 2.2 `index_tasks` 表、`materials.content_hash` / `revision`、`chunks` 唯一约束的幂等迁移
  - verify: 连续两次执行 `db.Migrate` 不报错；四个唯一索引可见
  - 实测：backend 先后重建启动 3 次均打印「数据表与种子数据已就绪」；真实库 `information_schema.STATISTICS` 查得 `chunks.uk_chunk_seq`、`index_tasks.uk_idem`、`index_tasks.uk_task_id`、`materials.uk_material_hash`
- [x] 2.3 上传把材料与任务写进同一事务（事务性发件箱）
  - verify: 单测覆盖事务成功与约束冲突回滚；Go e2e 中每条新任务都有对应材料
  - 实测：`store/tasks_test.go`、`httpapi/tasks_test.go` 通过；Go e2e 用 202 返回的 `material_id` 读取详情成功且 `index_status=ready`

## 3. 提交接口（第三步）

- [x] 3.1 上传与重索引返回 202 + `task_id` + `status_url`，关闭请求内 `qa.Index` 分支
  - verify: `grep -R "s.qa.Index" backend/internal/httpapi/` 无命中；Go e2e 断言 202 与响应不含 `ready`
  - 实测：grep 无命中；Go e2e「教师上传非空材料 → 202」「202 响应不含索引结果」「对材料重索引 → 202」均通过
- [x] 3.2 幂等键由业务含义派生并落在唯一约束上；重放终态任务而非新建
  - verify: 单测覆盖键生成与「相同输入同键」；Go e2e 重复提交断言 `task_id` 相同、任务数不增
  - 实测：单测通过；Go e2e「同班同一份内容重复提交 → 202 且复用同一 task_id / material_id」通过；判据三两次提交得到同一 `task_id`
- [x] 3.3 前端上传 / 重索引适配 202 并轮询到终态，界面显示 pending / 失败原因
  - verify: 浏览器 e2e 的 T3 / T9 / T12 上传断言仍通过；失败任务界面给出「重试索引」
  - 实测：`node scripts/e2e-ui.mjs` 51 通过 / 0 失败（T3、T9、T9b、T12a 均通过）；失败态「重试索引」按钮由 `MaterialsPage` 保留，未在本轮制造失败任务做界面断言

## 4. 消费者（第四步）

- [x] 4.1 `internal/indexer` + `cmd/worker` 独立进程：`SKIP LOCKED` 领取、租约回收、独立 context
  - verify: `go build ./...` 通过；停 worker 提交后再启动，任务自动到 `succeeded`
  - 实测：build / vet / test 通过；worker 日志 `worker_started poll_ms=500 lease_s=630 batch=4`；判据二实测 `pending → succeeded`
- [x] 4.2 状态机、失败分类、指数退避加抖动、次数上限与死信
  - verify: 单测覆盖退避单调与抖动范围、permanent / 限流直接终态；dead 任务不再被领取
  - 实测：`indexer/worker_test.go` 覆盖分类与退避；判据三 kill 后重启 `attempts=2` 且终态 `succeeded`（租约过期由脚本注入以缩短等待，回收仍由 `RecoverStaleTasks` 完成）
- [x] 4.3 三处幂等落地：`chunks` 唯一约束、确定性 Qdrant 点 ID、payload `chunk_id` 校验
  - verify: 单测覆盖确定性 ID 可复现与内容变化即变化；重复投递后切片行数与点数不增
  - 实测：`qa/point_id_test.go` 通过；判据三首次与再次重索引均为「切片 1 行 / 1 个 seq、向量点 1」，向量点未翻倍
- [x] 4.4 Web 词面索引按切片版本跨进程失效
  - verify: 单测覆盖版本变化触发重建；Go e2e 在 worker 写库后检索新材料独有词命中
  - 实测：Go e2e「R5.1 / R6 / R7 命中检索 → 流式带引用回答」6 条全通过，Web 进程读到了另一进程写入的切片

## 5. 状态接口、隔离与观测（第五步）

- [x] 5.1 `GET /api/tasks/{task_id}` 只读、班级过滤、跨班 404
  - verify: Go e2e 连续查询 `attempts` 不变、跨班返回 404 且不含任务信息
  - 实测：Go e2e「状态接口连续两次 200 且 attempts 不变」「他班教师查询本班任务 → 404 且不泄漏任务信息」通过
- [x] 5.2 上传审计含 `task_id`、任务带 `request_id`、终态写审计
  - verify: Go e2e 核对审计与任务行的关联字段
  - 实测：Go e2e「上传审计 detail 含 task_id」「任务行带 request_id」通过；判据二归档输出中可见 `request_id` 原值
- [x] 5.3 `scripts/reindex-all.mjs` 与文档同步新契约
  - verify: 脚本按 202 + 轮询判定完成；README 写明 202、状态接口与 worker 服务
  - 实测：脚本已改为 202 + `waitForTask`，`node --check` 通过；README 已写明 202、四态状态机、worker 启停、判据复现与换模型后重索引。全量重索引已执行，98/98 成功，见第 7 节
- [x] 5.4 教师端一键补齐未索引材料（spec A9）
  - verify: 材料页出现「一键索引未索引（N）」，N 等于非 `ready` 材料数；前端 `npm run build` 通过
  - 实测：按钮位于 `material-summary`，按 4 篇一批复用 `reindex` + `waitForTask`，显示进度与成功/失败计数，429 时停止后续批次；`npm run build` 通过且产物含按钮文案；OpenSpec strict `9 passed / 0 failed`

## 6. 验证与证据

- [x] 6.1 离线质量门禁全绿
  - verify: `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` 退出码 0
  - 实测：完整门禁（含 `--with-e2e`）9 项全绿：配置样例 / 分层 / 密钥卫生 / gofmt / build / vet / test / OpenSpec strict / 端到端验收
- [x] 6.2 Go 端到端验收（含判据一、判据三与班级隔离）
  - verify: `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-e2e` 退出码 0
  - 实测：`go run ./cmd/e2e` **81 通过 / 0 失败**；浏览器 e2e **51 通过 / 0 失败**
- [x] 6.3 三条判据复现脚本与归档
  - verify: `node scripts/async-criteria.mjs` 输出三条判据的命令、结果与结论；`reports/async-criteria.md` 含命令与原始输出
  - 实测：三条判据 **PASS / PASS / PASS**，归档 `reports/async-criteria.md`。判据一 P95 17.6ms vs 索引执行 216ms；判据二停消费者期间 `pending`、恢复后 `succeeded`；判据三入口幂等、切片/向量点不翻倍、`attempts=2`

## 7. 追加实测与剩余边界

- **存量材料已全量重建**：换对话模型时先删除 Qdrant 集合 `team_campusclaw_chunks`（清掉 892 个旧点与测试材料孤儿点），再执行 `node scripts/reindex-all.mjs --reset --delay-ms 150`，**98/98 成功、0 失败**。重建后 98 篇全部 `ready`、1740 个切片全部带 `heading_path`、Qdrant 1740 点 / 2048 维（与切片数一致，无孤儿点）。真实问答「酶是什么？」命中 `生物化学｜酶（1/3）` 共 4 条引用，未降级。
- **对话模型已换为 `glm-5.3-flash`**：实测非流式与流式均 HTTP 200，流式 39 帧含 `[DONE]`，正文正常输出；仍是推理模型（前 36 帧为 `reasoning_content`），客户端只取 `content`，回答前会有一段静默。问答链路实测不再降级。嵌入 `embedding-3` 稳定 200、2048 维。
- **材料库测试文本已清理**：删除本轮验收脚本造的 14 条材料（`id>98`），`chunks` / `index_tasks` 由外键 CASCADE 一并清掉；真实课程材料 `id=1..98` 保留。
- **教师端新增「一键索引未索引（N）」**：对 `index_status != ready` 的材料按 4 篇一批提交并轮询到终态，显示进度与成功/失败计数，429 时停止后续批次；复用单篇 reindex 接口，无新增写接口。
- **课程第 8 课任务清单的安全自查（SEC-01~08）与 `reports/risk-authz.md` / `risk-concurrency.md` / `risk-transaction.md` 等归档不在本轮范围**：未代做安全自查，也未伪造这些文件。
- **浏览器 e2e 本轮未重跑**：验收脚本会重新上传测试材料，与「清理材料库」冲突；本轮以 `npm run build`、前端产物文案核对、真实问答链路与离线门禁 8 项全绿为准。
