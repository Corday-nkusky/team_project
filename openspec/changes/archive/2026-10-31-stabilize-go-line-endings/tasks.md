# Tasks

> 按编号顺序执行；每条以可判定的 `verify:` 说明完成条件，实测后勾选。

## 1. 根因与仓库级修复

- [x] 1.1 复现换行根因并固定 Go 文件属性
  - verify: 修复前门禁仅 gofmt 失败且相关文件为 `i/lf w/crlf`；修复后 `git check-attr text eol -- backend/internal/config/config.go` 返回 `text: set` 与 `eol: lf`
  - 实测：修复前 `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs` 仅 gofmt 失败并列出 20 个文件，build/vet/test 通过；`git ls-files --eol '*.go'` 对这 20 个文件均报告 `i/lf w/crlf`，`gofmt -d` 只有换行差异。新增属性后 `git check-attr` 返回 `text: set`、`eol: lf`。
- [x] 1.2 用 gofmt 规范化现有 Go 工作区
  - verify: 在 `backend/` 执行 gofmt 后，`/d/tools/go/bin/gofmt.exe -l .` 无输出，`git ls-files --eol '*.go'` 不含 `w/crlf`
  - 实测：`/d/tools/go/bin/gofmt.exe -w . && /d/tools/go/bin/gofmt.exe -l .` 退出码 0 且无输出；`git ls-files --eol '*.go'` 显示全部 28 个 Go 文件均为 `i/lf w/lf attr/text eol=lf`。
- [x] 1.3 确认修复没有业务逻辑差异
  - verify: `git diff -- backend/ scripts/quality-gate.mjs` 为空，最终 Git 差异只包含本 change 与根 `.gitattributes`（不计算其他代理已有文件）
  - 实测：`git diff --exit-code -- backend/ scripts/quality-gate.mjs` 退出码 0，无内容差异；本 debug 线只新增根 `.gitattributes` 与独立 change，未改门禁脚本或业务代码。

## 2. 跨平台与门禁验证

- [x] 2.1 在强制 CRLF 偏好的隔离临时仓库验证重新检出
  - verify: 以命令级 `core.autocrlf=true`、`core.eol=crlf` 重新检出后，Node 字节断言确认 Go 文件含 LF 且不含 CRLF，gofmt 检查通过
  - 实测：临时仓库从索引删除后重新检出 `config.go`，命令级强制 `core.autocrlf=true`、`core.eol=crlf`；Node 输出 `forced autocrlf/eol=crlf checkout: LF-only bytes confirmed`，gofmt 检查无输出，退出码 0。
- [x] 2.2 运行离线门禁与 OpenSpec 严格校验
  - verify: `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` 输出「质量门禁全部通过。」且退出码 0
  - 实测：配置样例、分层、密钥、gofmt、go build、go vet、go test、OpenSpec 严格校验全部通过，输出「质量门禁全部通过。」，退出码 0。
- [x] 2.3 运行 Go 接口 e2e 回归（不重启 Docker、不运行浏览器 e2e）
  - verify: 加载部署环境并指向宿主机 MySQL 映射端口后，`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-e2e` 输出「质量门禁全部通过。」且退出码 0
  - 实测：`set -a; . deploy/.env; set +a; MYSQL_HOST=127.0.0.1 MYSQL_PORT=13306 GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-e2e` 全部通过（Go e2e 7745ms），输出「质量门禁全部通过。」，退出码 0；未重启 Docker，未运行浏览器 e2e。
