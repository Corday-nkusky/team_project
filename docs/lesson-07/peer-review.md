# 第七课同伴评审记录

> **状态：未签署。** 本表用于第二位团队成员逐项复核；在评审人填写并签字前，本变更不满足「完成定义」。
> 作者（Corday）不得自评或代填。

## 评审对象

- 变更：`openspec/changes/2026-10-28-lesson-07-baseline/`
- 版本：tag `v0.0.1-skeleton`（`git rev-parse v0.0.1-skeleton` 取得提交号）
- 评审日期：（待填）
- 评审人（姓名 / GitHub 账号）：（待填）
- 评审用时：（待填）

## 复现步骤（评审人自行执行）

```bash
git fetch --tags && git checkout v0.0.1-skeleton

# 1. 离线质量门禁
node scripts/quality-gate.mjs
#    Go 不在 PATH 时：GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs

# 2. 规约严格校验（需网络）
npx --yes @fission-ai/openspec@latest validate --all --strict

# 3. 一键启动与探活（需 Docker Desktop 已运行）
node scripts/up.mjs
curl -s http://127.0.0.1:8080/health

# 4. 端到端（服务已启动）
cd backend
MYSQL_HOST=127.0.0.1 MYSQL_PORT=13306 MYSQL_USER=campusclaw \
  MYSQL_PASSWORD='<来自 deploy/.env>' go run ./cmd/e2e
```

## 评审清单

- [ ] 仓库：公开可访问，协作者含三名成员，`main` 已配置保护（PR + 1 人评审）
- [ ] 规约：change 四件套齐全，名称带日期
- [ ] 规约：抽查 3 条 Scenario，GIVEN / WHEN / THEN 均可判定
- [ ] 规约：至少 4 条失败路径，且与 e2e 编号对得上
- [ ] 规约：能力↔场景对照表覆盖现有全部能力
- [ ] 规范：`AGENTS.md` 能直接指导一次真实改动
- [ ] 规范：八类规则均含检查方式、违规处理、有效性验证
- [ ] 规范：三个技能可在新会话中按步骤执行
- [ ] 骨架：`check-layering.mjs` 通过，且故意反向 import 会被拦
- [ ] 骨架：缺 `JWT_SECRET` 时进程非零退出，日志为 JSON 且不回显值
- [ ] 骨架：`/health` 免认证，含 `version` / `time` / `deps`，无密钥
- [ ] 骨架：访问日志为单行 JSON，`X-Request-ID` 可透传、非法值被替换
- [ ] 骨架：`node scripts/up.mjs` 一条命令起全套并 200
- [ ] 交付：`quality-gate.mjs` 与 e2e 退出码均为 0
- [ ] 交付：`docs/lesson-07/verification.md` 中的未验证项如实标注

## 评审意见

（待填：按文件与行为描述，注明严重级别 P0/P1/P2）

| 编号 | 位置 | 问题 | 级别 | 处理结论 |
| --- | --- | --- | --- | --- |
| | | | | |

## 结论

- [ ] 通过
- [ ] 有条件通过（遗留项需在合并前解决）
- [ ] 不通过（说明原因）

## 签字

- 评审人：＿＿＿＿＿＿＿＿　日期：＿＿＿＿＿＿＿＿
- 作者确认已处理全部意见：＿＿＿＿＿＿＿＿　日期：＿＿＿＿＿＿＿＿
