# Spec Delta

## Purpose

让 Go 格式门禁在不同操作系统和不同本机 Git 自动换行设置下得到一致、可复现的结果。

## ADDED Requirements

### Requirement: R12 Go 源码换行与格式一致性

仓库 SHALL 在版本控制的 `.gitattributes` 中将所有 `*.go` 文件声明为 `text eol=lf`，不得依赖开发者的全局或本地 `core.autocrlf`、`core.eol` 设置。工作区 Go 源码 SHALL 符合 gofmt 规范输出；质量门禁 SHALL 在存在格式差异时列出文件并以非零码结束。

#### Scenario: R12.1 启用自动 CRLF 时仍检出 LF

- **GIVEN** 临时仓库包含本变更的 `.gitattributes` 与 Go 源码，且检出命令设置 `core.autocrlf=true` 和 `core.eol=crlf`
- **WHEN** 从索引重新检出 Go 文件并读取其原始字节
- **THEN** 文件包含 LF 换行且不含 CRLF 换行
- **AND** 对该文件运行 gofmt 检查无格式差异

#### Scenario: R12.2 当前工作区通过格式与完整门禁

- **GIVEN** 仓库已应用 Go 换行策略并完成现有源码格式化
- **WHEN** 运行 `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec`
- **THEN** gofmt 步骤不列出任何文件
- **AND** 配置、分层、密钥、build、vet、test 与 OpenSpec 严格校验全部通过，命令退出码为 0

#### Scenario: R12.3 Go 格式差异仍被拒绝

- **GIVEN** 任一工作区 Go 文件与 gofmt 规范输出存在换行或格式差异
- **WHEN** 运行质量门禁
- **THEN** gofmt 步骤列出该文件
- **AND** 门禁最终退出码为 1，不因构建、vet 或单元测试通过而掩盖失败
