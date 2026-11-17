# CampusClaw 智教闭环 · 课程小组项目报告

> 仓库：https://github.com/Corday-nkusky/team_project
> 录屏（≤3 分钟）：**待上传**——网盘地址在录屏完成后写回本节；本轮未录制，原因见文末。
> 分工（git 作者标识，与提交历史逐条对得上）：

| 成员 | 负责 | 提交证据 |
| --- | --- | --- |
| Corday | 第 7 课基线、规范体系、e2e 验收 | 33 commits，如 `031552c` `c9a0ab0` |
| Corday-nkusky | LLM 网关、RAG 重写、异步索引、历史问答、技能与 MCP | 7 commits，`#2`～`#6`（`49b1c3a`…`bfeaa8a`） |
| Aria | 真实网关验收与跨平台 E2E | `8ce3a42`（2026-11-03） |

---

## 1. 一句话产品定位（10 分）

**面向班级教学、用本班材料作答并逐条给出原文出处的提问—复习—反馈闭环。**

系统从零自建，不基于既有系统增量：教师上传材料并切分入库，学生用带引用的流式问答提问，疑问与掌握状态回流到教师看板。代码入口 `backend/cmd/server/`，前端 `frontend/src/`，规约 `openspec/specs/`。

## 2. AI 写了什么（20 分）

**Harness 与模型**：WorkBuddy（Agent 模式），本轮会话模型 Tencent Hunyuan Hy；规则入口 `AGENTS.md` + `.qoder/rules/` 五类工程规范与三类按需补充，逐条列在 `AGENTS.md` §九。

AI 实际产出四类，均可查：

| 类别 | 具体产出 | 路径 |
| --- | --- | --- |
| 后端代码 | 技能加载与提示词叠加、MCP 客户端与宿主编排、三张表 DDL、7 组 handler、工具结果与指令过滤 | `backend/internal/skill/`、`backend/internal/mcpclient/`、`backend/internal/httpapi/{skills,mcp,ask-tools}.go`、`backend/internal/db/db.go` |
| 前端与接口页面 | 教师技能配置页、问答技能/工具状态、Swagger、H5 四步端 | `frontend/src/pages/SkillsPage.tsx`、`frontend/public/{swagger,h5}.html`、`frontend/public/openapi.json` |
| 规约 | 1 个 change 五件套 + 2 份主规约 + 旧 change 的 MODIFIED delta 同步 | `openspec/changes/2026-11-17-add-skills-and-mcp-tools/`、`openspec/specs/{skills,mcp-tools}/` |
| 证据报告 | 安全自查、修复对照、审计基线、3 份审核、3 份三查、先红后绿、可追溯矩阵 | `reports/`、`docs/traceability.md` |

**人做了什么**：逐段审核 AI 产出（不是只看能否跑）、确认判据与数值、复跑门禁、把未真机项标成未验证。准入流程和七类核查清单在 `AGENTS.md` §一之一。

## 3. 审核记录（25 分）

三份审核记录，每份写全"用什么手段 / 查了什么 / 判据 / 结果"：

| 文件 | 手段 | 结论 |
| --- | --- | --- |
| `reports/review-01.md` | 脚本门禁 + 单测 + AI 自审 | 可用（SEC-01～05 绿、SEC-06 红、SEC-07/08 黄） |
| `reports/review-02.md` | AI 自审 + 脚本门禁 + 1 条人工确认 | 技能与 MCP 新增代码可用 |
| `reports/review-03.md` | 门禁、CI 与交付链路复核 | 可用；main 保护待在 GitHub 设置 |

七类核查对象（接口入口 / 业务逻辑 / 数据库 / 公共函数 / 错误处理 / 安全隐私 / 性能）及各自检查手段在 `AGENTS.md:19-36`；三分法与"AI 初稿→逐段审核→交叉评审→合入 main"准入流程在同节。

**可复制复核**：

```bash
node scripts/make-check.mjs                 # 全量门禁
node scripts/make-check.mjs security        # 密钥 + vet + 全量单测
npx @fission-ai/openspec@latest validate --all --strict
```

实测：门禁 7 项全绿、`check-security` 全绿、OpenSpec `validate --all --strict`：归档前 `14 passed / 0 failed`，四个 change 归档后当前 `10 passed / 0 failed`。

## 4. 高风险三查（25 分）

| 风险 | 记录 | 判据与结果 |
| --- | --- | --- |
| 并发 | `reports/risk-concurrency.md` | 重复提交同一内容只出一份结果；唯一约束挡"先查后写" |
| 事务 | `reports/risk-transaction.md` | 停 consumer 时 pending，恢复后 succeeded，无"改了一半" |
| 权限 | `reports/risk-authz.md` | B 班账号读 A 班讲义 404，与"不存在"同形；学生写操作 403 |

三份都按四件事写全：手段、查了什么、判据、结果；依据是 `reports/async-criteria.md` 的真实输出与 e2e 401/403/404 负向断言。

## 5. 门禁与演示（20 分）

**门禁**：`Makefile` 提供 `make check` / `make check-security`；本机无 `make`，等价入口 `scripts/make-check.mjs` 已实跑退出码 0。CI 在 `.github/workflows/quality-gate.yml`，与本地同一条命令。先红后绿证据 `reports/gate-red-green.md`：基线绿 0 → 注入假 `sk-` 密钥红 1 → 移除后绿 0。依赖审计已跑并逐条登记于 `reports/security-baseline.md`（Go 12 条可达来自工具链 `go1.27.1`，npm 2 包含 1 high，均保留红，不写"已修复"）。

**2026-11-17 真机实测**（`node scripts/up.mjs` 从空 Compose 起五服务，`/health` 返回 `deps={"mysql":"ok","qdrant":"ok"}`）：

| 验证 | 实测结果 | 原始输出 |
| --- | --- | --- |
| H5 四步链路 | 全绿；序列 `pending → running → succeeded`，跨班 404 且响应不含 B 班密语 | `reports/verify-h5.log` |
| 接口测试 | **42/42 通过**，含端到端技能→工具→调用记录；报告 `apitest/report.html` | `reports/apitest-run.log` |
| 完整 Go e2e | **81 通过 / 0 失败** | `reports/e2e-run.log` |
| 真实模型网关 | 全部通过；SSE 74 个 delta、2 条引用、回答 133 字 | `reports/verify-llm.log` |
| 启动与探活 | 五服务 healthy；`/swagger.html`、`/openapi.json`、`/h5.html` 均 HTTP 200，OpenAPI 16 路径 | `reports/run-up.log` |
| 并发 / 事务判据 | 两条均 PASS：重复提交复用同一任务、切片与向量点不翻倍；停 worker 时 pending、恢复后 succeeded | `reports/criterion-3.log`、`reports/criterion-2.log` |
| 技能开关 | 关闭给定义、开启只给引导，`meta.skill` 同步变化 | `reports/skill-toggle.md` |
| 注入反例 | SQL 注入 401/400、XSS 详情转义、CSRF 401，三条全绿 | `reports/sec08-run.log` |
| 压测基线 | 提交 C=4 达 419 req/s、查询 C=4 达 1969 req/s；检索 P95 24～64 秒，瓶颈在模型网关 | `reports/bench.md` |

**演示命令**（演示机器按序跑）：

```bash
git clone https://github.com/Corday-nkusky/team_project.git && cd team_project
node scripts/up.mjs                                  # 一键起五服务
curl http://127.0.0.1:8080/health                    # 依赖连通
node scripts/verify-h5.mjs                           # 202 + task_id → 轮询 → 跨班 404
python apitest/run_all.py                            # 按 URL 组织的接口用例 + HTML 报告
node scripts/make-check.mjs && node scripts/make-check.mjs security
```

**录屏**：待补。内容按上面命令顺序录，开头介绍、等待快进、结尾登录走核心功能；上传后把网盘地址写回开头。

## 6. 复盘（不计分）

**代价最大的一次返工：RAG 检索整条链路重写**（`openspec/changes/2026-11-10-rebuild-rag-pipeline/`）。

- **原因**：不是参数没调好，是做法不成立——固定 500 字窗口切片丢标题层级；每次提问把全班 `ready` 切片读进 Go 内存逐片打分；两路分数按 `0.6/0.4` 硬乘；"有依据"写死成命中 2 个内容词。原文见该 change 的 `proposal.md` §Why。
- **前后**：前＝固定 500 字 + 全表拉取 + 0.6/0.4 加权 + 内容词≥2；后＝200–300 字结构切片（带 `heading_path`）+ BM25 倒排 + 两路 Qdrant + RRF(`k=60`) + 本地二阶段精排 + 查询级/片段级证据判定 + 零密钥降级。
- **代价**：改动 6 个核心文件（`retrieval/{retrieval,index,rank}.go`、`qa.go`、`store.go`、`db.go`）；主规约 R5 新增 R5.6–R5.9；历史材料重索引 **56/98 篇**后因 embedding 网关当日额度耗尽中断，Go e2e 与浏览器 e2e 当轮标为未验证。合入提交 `bfeaa8a`（`#6`）。
- **教训**：先判定链路做法是否成立，再调参数；调 `limit`、阈值、把 2 降到 1 都不动主干。

---

## 本篇没能验证的部分

- **录屏与网盘地址**：未录制，未上传。
- **手机浏览器实测**：H5 四步由 Node `fetch` 复现同一请求序列并全绿，未在真实手机浏览器点按；`h5.html` 页面已在容器内验证可访问且默认账号已改为有上传权限的教师。
- **干净目录启动**：`up.mjs` 从空 Compose 起了五服务，但沿用了既有 MySQL / Qdrant 数据卷，不是全新目录；跨机器换目录未测。
- **交叉评审签字**：`docs/lesson-07/peer-review.md` 15 项全空，属队友项。
- **公网地址**：未确认部署方式，见 TODO 9.7。
- **观测面板三张图与告警真触发**：未做。压测基线已产出（`reports/bench.md`），但课程要求的"至少一项改进后用同脚本同参数复测"未做，压测项不算全绿。
- **依赖升级回归**：Go ≥ `go1.27.4` 与 `vite@8.3.8` 未升级，SEC-06 保持红。
- **XSS / CSRF 浏览器侧判定**：三条注入 payload 已真机绿（`reports/sec08-run.log`），但未做浏览器渲染截图；本系统用 Bearer 令牌、无会话 cookie，SameSite 不适用。
