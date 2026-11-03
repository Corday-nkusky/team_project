# Design

## Context

2026-10-31 基线实测中，`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs` 只有 gofmt 失败，列出 20 个文件；go build、go vet、go test 均通过。`git ls-files --eol '*.go'` 显示这些文件为 `i/lf w/crlf`，仓库索引本身已经是 LF。`gofmt -d internal/config/config.go` 删除 CRLF 行并新增内容相同的 LF 行，没有语法或逻辑差异。

根因是仓库未声明 Go 文件换行策略，工作区格式受开发者机器的 Git 自动换行设置影响。仅执行一次 `gofmt -w .` 能修当前工作区，但下一次 Windows 检出仍会复发。

## Goals / Non-Goals

**Goals:**

- 让 Go 源码在启用自动 CRLF 转换的 Windows 环境也以 LF 检出。
- 保留 gofmt 对真实格式错误的拒绝能力。
- 用最小差异修复当前红项，并证明没有业务逻辑变化。

**Non-Goals:**

- 不引入新的格式工具或第三方依赖。
- 不改质量门禁脚本，不忽略 CRLF 差异。
- 不处理前端或与本问题无关的工程整改。

## Decisions

### D1 用仓库级 `.gitattributes` 固定 Go 文件为 LF

在根目录声明 `*.go text eol=lf`。`text` 明确按文本处理，`eol=lf` 同时约束工作区检出为 LF；该规则随仓库分发，优先于本机自动换行偏好，且根目录无斜杠模式可匹配各级目录中的 Go 文件。

被放弃的候选方案：

- **要求每位开发者设置 `core.autocrlf=false`**：依赖本机状态，新环境与 CI 不可保证，且会改变其他文件的既有行为。
- **让门禁忽略 CRLF 或先自动格式化再检查**：会掩盖工作区与规范输出的差异，削弱「退出码即判定」。
- **只运行一次 gofmt**：不能防止新检出复发。

### D2 用 gofmt 规范化当前工作区，不手工改业务代码

在 `backend/` 运行 `gofmt -w .`。随后同时检查 `gofmt -l .` 无输出、`git ls-files --eol '*.go'` 不再出现 `w/crlf`，以及 `git diff -- backend/` 不含业务代码差异。由于索引已是 LF，本次纯换行规范化不应产生 Go 内容补丁。

### D3 用隔离的临时仓库验证新检出行为

将新 `.gitattributes` 与一个 Go 文件放入临时仓库并加入临时索引，删除临时工作文件后，以命令级 `core.autocrlf=true`、`core.eol=crlf` 从索引重新检出。断言检出的字节包含 LF 且不含 CRLF。该验证不修改仓库配置，也不触碰正式工作区文件。

失败路径沿用现有质量门禁：只要 Go 文件与 gofmt 规范输出不同，gofmt 步骤列出文件并最终非零退出；本变更不增加豁免。

## Risks / Trade-offs

- **已有工作区不会因新增属性立即换行**：必须同时执行一次 gofmt，并用 `git ls-files --eol` 验证。
- **Go 文件在 Windows 编辑器中显示为 LF**：这是 gofmt 的规范形态；仓库级策略比各编辑器自动转换更稳定。
- **属性只覆盖 Go 文件**：有意避免批量改变脚本、文档和前端文件的换行行为。
