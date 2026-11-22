# Proposal: 修复镜像版本烟雾检查

## Why

维护者需要 `image-version` 准确验证运行镜像中的提交版本。当前 CI 在没有 MySQL/Qdrant 的 job 中沿用 `REQUIRE_DEPS=true`，容器等待依赖而不监听端口，导致所有后续 PR 被既有红色检查阻塞。

- 谁的问题：提交者与评审者无法凭该检查区分镜像版本问题和依赖配置问题。
- 不做会怎样：主干与 PR 持续失败，版本注入即使正确也无法通过 CI 验证。
- 做完如何验证：本地以 CI 同参数构建并启动镜像，`/health` 返回降级状态和注入版本；工作流通过严格校验，失败时能输出容器日志。

## What Changes

- 仅在镜像版本 smoke 容器中设置 `REQUIRE_DEPS=false`，允许无依赖启动并核对 `/health.version`。
- 验证失败时、删除容器前输出容器日志，保留可诊断信息。
- 同步 CI 文档，明确该 smoke 的 503 降级响应是预期行为。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `final-acceptance`：补足版本镜像烟雾检查的依赖前置条件与失败诊断要求。

## Non-goals

- 不改变生产、Compose 和本地开发的默认依赖策略。
- 不为该版本检查额外启动 MySQL/Qdrant，也不把 503 降级响应宣称为完整链路验收。
- 不修改业务代码、镜像版本注入方式或其他 CI 门禁。
