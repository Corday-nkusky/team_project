---
name: run-quality-gate
description: 运行 CampusClaw 的质量门禁并处理失败项：分层依赖、密钥卫生、配置样例、gofmt、go build/vet/test，可选 OpenSpec 严格校验与端到端验收。适用于提交前自检、评审前验证、CI 失败排查或依赖升级后的回归。
---

# 运行质量门禁

## 命令

```bash
# 离线全量（默认）
node scripts/quality-gate.mjs

# Go 不在 PATH
GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs

# 追加 OpenSpec 严格校验（需要 npx 网络）
node scripts/quality-gate.mjs --with-spec

# 追加接口端到端（需要 MySQL / Qdrant / 后端已启动）
GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-e2e
```

退出码即结论：0 全绿，非 0 有失败项。脚本会打印失败步骤与原始输出。

## 步骤与含义

| 步骤 | 拦什么 | 常见修复 |
| --- | --- | --- |
| 配置样例完整性 | `.env.example` 缺键、compose 没把 JWT_SECRET 设必填 | 补键；改 `${JWT_SECRET:?…}` |
| 分层依赖 | 反向 import、httpapi 直连 llm/qdrant/retrieval、新包未登记 | 调整落点或登记层级 |
| 密钥与配置卫生 | 真实密钥形态、`.env` 入库、默认密钥回落 | 删默认值、轮换密钥 |
| gofmt | 未格式化文件 | `gofmt -w .` |
| go build | 编译错误 | 按报错修 |
| go vet | 可疑代码（printf、锁、空指针） | 按报错修 |
| go test | 单测失败 | 先看断言实际值，再改实现或测试 |
| OpenSpec（可选） | 场景格式、重复需求、无效 change | 修 spec 后重跑 |
| e2e（可选） | 接口行为回归 | 对照 Requirement 编号定位 |

## 纪律

- **不得绕过**：禁止 `--no-verify`、禁止为过门禁而删除或弱化断言。
- 失败先读原始输出，不要凭猜测改代码。
- 修完后重跑同一条命令，确认退出码为 0。
- 若 Docker 未运行导致 e2e 跑不了，记录「未验证」并说明原因，不得声称通过。

## 完成标准

门禁输出末尾为「质量门禁全部通过。」且退出码 0；把命令与实际输出摘要写进 tasks 的「实测」。
