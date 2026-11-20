# TODO — CampusClaw 最终验收清单

> 更新：2026-11-21。只记录当前真实状态；历史过程与原始输出以 `reports/`、OpenSpec tasks 和提交历史为准。
> 本轮不生成视频，不填写虚构网盘链接，不把公网、真实用户、手机浏览器、同伴签字或远端 Actions 写成已完成。

## 已完成并有本机证据

- [x] 干净目录 `D:/code/team-project-clean-smoke-20261121`、新 Compose 项目与新数据卷一条命令启动五服务；`/health` 200，MySQL/Qdrant 全 ok。证据：`reports/clean-up.log`。
- [x] 质量门禁、配置样例、分层、密钥、gofmt、build、vet、Go 单测与 OpenSpec strict 全绿。证据：`reports/quality-gate-latest.log`。
- [x] 统一安全门禁覆盖密钥、依赖、SQL/XSS/CSRF；Go 可达漏洞 0、npm 漏洞 0，故意破坏审计命令会非零退出。证据：`reports/security-gate-latest.log`、`reports/security-gate-intentional-red.log`。
- [x] Go 接口 e2e 81/81，apitest 42/42，H5 四步全绿。证据：`reports/e2e-run-latest.log`、`reports/apitest-run-latest.log`、`reports/verify-h5-latest.log`。
- [x] 15 题问答集在干净数据卷同断言复跑：有答案、无答案、跨班各 5/5；保留跨班题先红记录与检索修复。证据：`reports/eval-15-clean-red.log`、`reports/eval-15.log`。
- [x] 七列审计、additive migration、学生 403 留痕、教师三类真实观测图及加载/空/错误/正常四态。证据：`reports/audit-seven-columns.json`、`reports/migration-schema.log`、`reports/observability-page.png`。
- [x] 告警真实持续越阈并送达；高阈值未触发与接收器失败分别留证。证据：`reports/alert-*.log`、`reports/alert-delivery.json`。
- [x] 材料列表缓存同镜像同参数 A/B：C=1/4/8 吞吐提升约 74.4%/50.3%/19.9%，错误均 0；未宣称所有指标改善。证据：`reports/bench.md`。
- [x] 镜像版本可由 `/health` 反查；CI 配置与本机命令一致。证据：`reports/image-version-smoke.json`、`.github/workflows/quality-gate.yml`。
- [x] `report.md` 六节与 `docs/final-presentation.md` 五分钟汇报稿已成稿。

## 本轮收口

- [x] 完整浏览器回归 56/56：键盘焦点、真实上传/问答、阅读与观测四态、学生权限均通过。证据：`reports/e2e-ui-latest.log`。
- [x] 干净实例已重跑异步判据并恢复 worker：判据二 PASS；零密钥本地执行过快使判据一 FAIL，向量与重复投递窗口未验证的判据三 SKIP。原始输出：`reports/async-criteria-clean-zero-key.md`。
- [x] 原实例以当前代码和已配置模型模式复跑异步三判据全 PASS：P95 21.8ms vs 执行 327ms，恢复不丢任务，切片/向量不重复且重投 `attempts=2`。证据：`reports/async-criteria.md`。
- [x] 已用最终前端源码重建干净实例并完成回归；`report.md` 已回填 56/56 与证据路径。
- [x] 最终质量、安全、OpenSpec strict 与前端构建均退出 0，OpenSpec 4.2/4.4/4.5 已按实测回填；报告无结果占位符。证据：`reports/quality-gate-latest.log`、`reports/security-gate-latest.log`、`reports/frontend-build-latest.log`。
- [x] 干净实例已停止，原 `deploy` 五服务已用当前源码重建并恢复，`/health` 200 且 MySQL/Qdrant 全 ok；未删除数据卷。证据：`reports/restore-original.log`。

## 需要外部条件，保留未完成

- [ ] 真实公网部署与可访问地址。
- [ ] 至少三名真实使用者反馈及每条去向。
- [ ] 真实手机浏览器四步验证；目前仅有本机 H5 接口链路证据。
- [ ] 第二位成员评审签字与组内从零启动交叉验证；不由作者代签。
- [ ] 远端 GitHub Actions 实际运行与 main 保护规则截图；本机配置检查不能替代。
- [ ] 演示录屏与网盘地址。本轮明确不生成视频，因此不创建录屏脚本、不伪造链接。

## 已知验证边界

- 本机无 gcc，缓存普通并发单测已通过，但 `go test -race` 未执行。
- 依赖审计仍如实保留“所需模块中 1 条当前代码不可达漏洞”的原始说明；可达漏洞为 0。
- 课程报告中的每条数字都必须能在上述路径找到原始输出；没有证据的项目继续标未完成。
