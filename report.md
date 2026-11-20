# CampusClaw 智教闭环 · 课程小组项目报告

> 仓库：https://github.com/Corday-nkusky/team_project  
> 录屏 / 网盘：未录制、未上传，按本轮决定不生成视频，不填写虚构链接。  
> 分工按 git 作者标识：Corday——规范、基线与最终验收；Corday-nkusky——LLM、RAG、异步索引、技能与 MCP；Aria——真实网关与跨平台 E2E（`8ce3a42`）。

## 1. 一句话产品定位（10 分）

**教师上传本班材料，学生只在本班资料中提问并获得带原文引用的流式回答；无依据时固定回答“资料中未找到相关内容”。** 系统从零实现认证、班级隔离、异步索引、技能、MCP、审计与观测；入口为 `backend/cmd/server/`、`frontend/src/`。公网地址尚未部署确认。

## 2. AI 写了什么（20 分）

Harness 为 **Qoder Agent**；会话未固定一个可核验的模型版本，不编造型号。AI 产出与人审入口如下：

| 产出 | 具体内容与路径 |
| --- | --- |
| 后端与数据 | Go REST/SSE、检索与技能/MCP、additive migration、七列审计：`backend/internal/` |
| 前端 | 材料、问答、技能和真实运行观测：`frontend/src/` |
| 规约 | Requirement + GIVEN/WHEN/THEN、失败路径与任务：`openspec/` |
| 测试与运维 | 质量/安全门禁、接口和浏览器 e2e、15 题、告警、压测、Compose：`scripts/`、`deploy/` |

人负责逐段审核、复跑命令、核对数值和标记未验证项；七类核查清单及“可用 / 待修改 / 风险”三分法在 `AGENTS.md` §一之一。

## 3. 审核记录（25 分）

| 记录 | 手段与结论 |
| --- | --- |
| `reports/review-01.md` | 门禁、单测与 AI 自审；保留安全红项并复测 |
| `reports/review-02.md` | 技能/MCP 逐层审核，端到端调用与拒绝路径可用 |
| `reports/review-03.md` | 门禁、CI 和交付链路复核；外部保护规则不冒充已配置 |

本轮将 Go 可达漏洞 12→0、npm 漏洞 2→0，三条 SQL/XSS/CSRF 反例常驻（`reports/security-baseline.md`）。15 题先在干净卷暴露通用“课堂标记”措辞造成无关片段误判；保留 `eval-15-clean-red.*`，补回归并修证据判定后同断言 15/15，而非删题碰绿。

## 4. 高风险三查（25 分）

| 风险 | 判据与实测 |
| --- | --- |
| 并发 | 同班同内容重复上传复用任务，切片与向量点不翻倍；唯一约束而非“先查再写”。`reports/risk-concurrency.md` |
| 事务 | 材料与任务同事务；停 worker 为 pending、恢复后 succeeded，无半条写入。`reports/risk-transaction.md` |
| 权限 | 查询带 `class_id`，跨班统一 404，学生写教师接口 403；观测拒绝也留审计。`reports/risk-authz.md`、`reports/observability-api.json` |

## 5. 门禁与演示（20 分）

`node scripts/quality-gate.mjs --with-spec` 与 `node scripts/security-gate.mjs` 均退出 0；CI 配置 check、check-security、image-version 三 job，本机镜像版本可经 `/health` 反查，远端 Actions 未冒充已运行。

| 验证 | 最终结果与证据 |
| --- | --- |
| 干净启动 | 新目录、新 Compose 项目、新数据卷，一条命令起五服务，MySQL/Qdrant 全 ok：`reports/clean-up.log` |
| 接口 / H5 | Go e2e **81/81**，apitest **42/42**，H5 四步全绿：`reports/*-latest.log` |
| 浏览器 | 真实 Chrome **56/56**，含键盘焦点、上传/问答、阅读四态、观测四态与学生 403：`reports/e2e-ui-latest.log` |
| 15 题 | 有答案 / 无答案 / 跨班各 **5/5**，核对引用与 `prompt_version`：`reports/eval-15.log` |
| 异步三判据 | 当前代码三项全 PASS：提交 P95 **21.8ms** vs 执行 **327ms**；停/恢复不丢任务；切片与向量不重复，故障重投 `attempts=2`：`reports/async-criteria.md` |
| 性能改进 | 按班两秒缓存、写后失效；同参数 C=1/4/8 吞吐提升 **74.4% / 50.3% / 19.9%**，错误均 0，C=8 P95 仍 4ms：`reports/bench.md` |
| 告警 | 连续两次真实 P95 越阈后送达，高阈值不触发、接收器失败退出 1：`reports/alert-*` |

复跑入口：`node scripts/up.mjs`、`node scripts/verify-h5.mjs`、`python apitest/run_all.py`、`node scripts/e2e-ui.mjs`、`node scripts/evaluate-qa.mjs`。

## 6. 复盘（不计分）

代价最大的返工是 RAG 链路重写：固定 500 字切片、全表打分和硬加权，改成结构切片、BM25、向量、RRF 与精排；历史重索引 56/98 篇时遇额度中断，当轮未把 E2E 写成通过。教训是先判断做法是否成立，再调参数；本轮通用措辞误判再次证明要沿检索与证据逻辑定位失败。

仍缺真实公网、三名真实用户反馈、真实手机浏览器、同伴签字及网盘录屏；均需要外部人员或账号，未伪造。`-race` 因本机无 gcc 未执行，普通并发单测已通过。零密钥干净实例的异步对照中，本地执行仅 10–19ms，判据一未满足“提交远快于执行”，向量与重复投递子项未完整验证；失败与 SKIP 原样保存在 `reports/async-criteria-clean-zero-key.md`，没有被当前已配置模式的三项 PASS 覆盖。
