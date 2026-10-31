# Proposal

## Why

Windows 工作区当前由系统级 `core.autocrlf=true` 把 20 个 Go 文件检出为 CRLF，而仓库没有 `.gitattributes` 约束 Go 源码换行。gofmt 以 LF 生成规范输出，因此这些文件即使业务代码已格式化，`gofmt -l .` 仍列出全部 20 个文件，质量门禁稳定以退出码 1 结束。

价值三问：

- **谁的问题**：在 Windows、macOS、Linux 与 CI 之间协作的仓库维护者；他们需要同一个可复现的格式门禁。
- **不做会怎样**：Windows 新检出会重复出现 gofmt 红项，真实格式问题会被换行噪声淹没，并诱使开发者修改本机 Git 配置或绕过门禁。
- **做完如何验证**：在强制启用 `core.autocrlf=true`、`core.eol=crlf` 的临时检出中，Go 文件仍只有 LF；当前工作区 `gofmt -l .` 无输出，完整质量门禁退出码为 0。

## What Changes

1. 在仓库根目录新增 `.gitattributes`，以版本控制配置声明所有 `*.go` 文件使用 `text eol=lf`。
2. 对 `backend/` 执行 gofmt，仅规范化现有 Go 源码格式与换行，不调整业务逻辑。
3. 为 `runtime-baseline` 增加跨平台 Go 源码格式要求及失败路径，保留现有门禁的非零退出判定。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `runtime-baseline`: 新增 R12，要求 Go 源码换行策略随仓库分发，并要求 gofmt 门禁在不同操作系统上得到一致结果。

## Non-goals（本次不做）

- 不修改全局或本地 Git 配置，不要求成员统一本机 `core.autocrlf`。
- 不修改、跳过或弱化 `scripts/quality-gate.mjs` 的 gofmt 判定。
- 不规范化非 Go 文件，不重写 Git 历史。
- 不修改前端、浏览器 e2e、接口行为或任何业务逻辑。

## Impact

- **工程配置**：新增根 `.gitattributes`。
- **源码**：仅现有 Go 文件的换行/格式规范化；Go 构建、测试与接口契约不变。
- **验证**：增加强制 CRLF 偏好下的临时检出检查，并复跑离线、OpenSpec 与 Go 接口门禁。
