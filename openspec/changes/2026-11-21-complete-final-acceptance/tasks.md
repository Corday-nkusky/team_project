# Tasks: 完成第 11 课最终验收证据链

## 1. 安全与 CI

- [x] 1.1 升级 Go 工具链声明与前端构建依赖，govulncheck/npm audit 高危为 0。verify: 两个审计命令退出码 0，原始输出归档。
  - 实测：2026-11-21 `GO_BIN=/d/tools/go/bin/go.exe node scripts/security-gate.mjs` 退出 0；Go 可达漏洞 0、npm 漏洞 0，见 `reports/security-gate-latest.log`。
- [x] 1.2 安全门禁接入密钥、依赖审计与安全反例。verify: `node scripts/make-check.mjs security` 退出码 0；故意制造审计失败时非零。
  - 实测：统一入口 `scripts/security-gate.mjs` 全绿，SQL/XSS/CSRF 三反例通过；`GOVULNCHECK_BIN=node` 故意破坏审计命令后最终非零，见 `reports/security-gate-intentional-red.log`。
- [x] 1.3 CI 并列执行 check/check-security 并构建版本镜像。verify: 工作流配置有效，镜像 `/health` 版本可反查提交。
  - 实测：`.github/workflows/quality-gate.yml` 配置三个 job；本机注入 `local-final-smoke-20261121` 后 `/health.version` 完全一致，见 `reports/image-version-smoke.json`。该 smoke 故意不启依赖，`degraded` 为预期；远端 Actions 未冒充已运行。

## 2. 审计与观测

- [x] 2.1 additive migration 增加审计七列并提供教师查询。verify: 单测覆盖七列、403、跨班和查询失败。
  - 实测：真实旧库重复启动无迁移错误，补列与 `idx_audit_class_action` 已回查，见 `reports/migration-restart.log`、`reports/migration-schema.log`；`internal/httpapi` 观测单测通过。
- [x] 2.2 问答与拒绝路径写技能、用量、耗时和拒绝记录。verify: 真实请求后按技能与操作人回查一致。
  - 实测：`GET /api/audit` 返回 200，真实问答含操作人、对象、技能、用量 1、耗时 4872ms，见 `reports/audit-seven-columns.json`；学生访问观测接口 403 后有 `forbidden/denied` 留痕。
- [x] 2.3 新增观测接口和教师三类图页面。verify: 单测/浏览器检查真实数据、空态、错误态和学生 403。
  - 实测：真实 Chrome 的 T16a–T16d（真实、加载、错误、空）与 T14a（学生无入口且 API 403）均通过；截图 `reports/observability-page.png`，输出 `reports/e2e-ui-latest.log`。
- [x] 2.4 提供告警持续条件和本地通知验证。verify: 真触发一次并保存送达记录；未达条件和接收器失败不冒充成功。
  - 实测：故意将 P95 阈值设为 0，连续 2 次读取真实指标后触发且接收器回执 `delivered=true`，见 `reports/alert-triggered.log`、`reports/alert-delivery.json`；高阈值未触发和接收器失败分别见对应日志。

## 3. 性能与评测

- [x] 3.1 实现按班隔离、写后失效的材料列表缓存。verify: 单测覆盖跨班、上传/重索引失效与并发安全。
  - 实测：`go test ./internal/config ./internal/httpapi` 通过；缓存测试覆盖班级隔离、克隆、TTL、单班失效与 8 goroutine 并发。因本机无 gcc，`-race` 未执行，未写成竞态检测通过。
- [x] 3.2 同脚本同参数完成改进前后压测。verify: `reports/bench.md` 两轮数字齐，错误为 0，命令可复跑。
  - 实测：同镜像同参数仅改 TTL；C=1/4/8 吞吐提升约 74.4%/50.3%/19.9%，错误均 0，C=8 P95 仍 4ms。原始输出 `reports/bench-{before,after}-cache.log`。
- [x] 3.3 登记基础提示词版本并执行 15 题评测集。verify: SSE 元数据有版本；评测输出三类题目计数与失败详情。
  - 实测：SSE `meta.prompt_version=grounded-qa:v1`；干净卷首轮跨班题因通用课堂措辞误判而红，补回归并修证据判定后，同断言复跑有答案/无答案/跨班各 5/5，见 `reports/eval-15-clean-red.log`、`reports/eval-15.log`。

## 4. 最终交付

- [x] 4.1 干净目录和新数据卷一条命令启动。verify: `/health` 200、依赖全通，保存命令输出。
  - 实测：`D:/code/team-project-clean-smoke-20261121` 排除 `.git`、依赖、配置与报告后，`COMPOSE_PROJECT_NAME=campusclaw-clean-20261121 node scripts/up.mjs` 退出 0；五服务运行，`/health` 为 200 且 MySQL/Qdrant 全 ok。原 `deploy_*` 卷保留，输出 `reports/clean-up.log`。
- [x] 4.2 完整门禁、接口、浏览器、H5、异步、安全与技能/MCP回归。verify: 各命令退出码 0，原始输出归档。
  - 实测：质量与安全门禁均退出 0；Go e2e 81/81、apitest 42/42、H5 四步、真实 Chrome 56/56 全绿；当前代码异步三判据全 PASS（P95 21.8ms vs 327ms，重投 attempts=2），见 `reports/async-criteria.md`；技能/MCP 审核与真实调用见 `reports/review-02.md`。零密钥干净对照的判据一 FAIL 与判据三 SKIP 单独保留，不冒充通过。
- [ ] 4.3 生成真实操作与验证命令录屏。verify: 视频可播放、时长不超过 180 秒，内容与本报告一致。
  - 实测：按用户本轮明确决定不生成视频，未录制、未上传，不填写虚构链接；本项保留未完成。
- [x] 4.4 压缩报告六节并准备 5 分钟汇报。verify: 报告一到两页、路径可打开；无伪造链接或签字。
  - 实测：`report.md` 为六节，`docs/final-presentation.md` 按 0:00–5:00 分六段；17 个报告引用证据路径逐一检查均存在，公网、真实用户、手机、同伴签字、远端 Actions 与录屏均明确保留未完成。
- [x] 4.5 运行最终质量门禁与 OpenSpec strict。verify: 退出码 0；未满足外部条件的事项在结论中明确列出。
  - 实测：`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` 退出 0，含 OpenSpec strict；`node scripts/security-gate.mjs` 与前端 `npm run build` 均退出 0，见 `reports/quality-gate-latest.log`、`reports/security-gate-latest.log`、`reports/frontend-build-latest.log`。报告明确列出公网、真实用户、真实手机、同伴签字、远端 Actions 与录屏未完成。
