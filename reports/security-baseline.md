# 依赖与密钥审计基线（SEC-05 / SEC-06）

> 2026-11-17 实测。事实分级：【实测】本机命令输出 /【代码】从实现可确认 /【未验证】没有证据。
> 原始输出归档：`reports/audit-govulncheck.txt`（327 行）、`reports/audit-npm.json`（208 行 JSON）。
> 判定口径：高危**清零**才转绿；未清零但逐条登记处置方式，仍判红（第 8 课要求保留至少一条红）。

---

## 1. 命令与环境【实测】

| 项 | 值 |
| --- | --- |
| 日期 | 2026-11-17 |
| Go | `/d/tools/go/bin/go.exe`，`go1.27.1`（`go.mod` 声明 `go 1.27.1`） |
| govulncheck | 本机未预装，用 `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` 在 `backend/` 执行 |
| Node / npm | v22.22.2 / 10.9.7，在 `frontend/` 执行 |
| npm registry | 默认 `registry.npmmirror.com` 的 audit 接口返回 404 `[NOT_IMPLEMENTED]`，改用 `--registry=https://registry.npmjs.org` 后拿到完整报告 |

复跑命令（原样可复制）：

```bash
cd backend && /d/tools/go/bin/go.exe run golang.org/x/vuln/cmd/govulncheck@latest ./...
cd frontend && npm audit --registry=https://registry.npmjs.org --json
node scripts/check-secrets.mjs
```

---

## 2. SEC-05 密钥 / 配置卫生【实测】

```text
✓ 密钥/配置卫生检查通过（扫描 225 个已跟踪文件）
```

退出码 0，0 命中。先红后绿的注入与恢复输出见 `reports/gate-red-green.md`。

---

## 3. SEC-06 Go 依赖审计【实测】

`govulncheck` 退出码 1（`exit status 3`）。原始汇总行：

```text
Your code is affected by 12 vulnerabilities from the Go standard library.
This scan also found 1 vulnerability in packages you import and 3
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
```

### 3.1 可达的 12 条（全部来自 Go 1.27.1 标准库）

| # | 编号 | 标题 | 发现于 | 修复于 |
| --- | --- | --- | --- | --- |
| 1 | GO-2026-6859 | HTTP/1 server crash due to MaxBytesReader header race | `net/http@go1.27.1` | `go1.27.4` |
| 2 | GO-2026-6809 | Distrust compromised intermediate certificates | `crypto/x509@go1.27.1` | `go1.27.3` |
| 3 | GO-2026-6617 | HTTP/2 server crash due to HPACK encoder race | `net/http/internal/http2@go1.27.1` | `go1.27.2` |
| 4 | GO-2026-6613 | HTTP/1 server connection desynchronization after 2xx CONNECT | `net/http@go1.27.1` | `go1.27.2` |
| 5 | GO-2026-6612 | Double flow control refund on HTTP/2 server streams | `net/http/internal/http2@go1.27.1` | `go1.27.2` |
| 6 | GO-2026-6611 | Excessive CPU consumption from repeated initial window changes | `net/http/internal/http2@go1.27.1` | `go1.27.2` |
| 7 | GO-2026-6610 | HTTP/2 transport accepts malformed framing-related headers | `net/http@go1.27.1` | `go1.27.2` |
| 8 | GO-2026-6609 | Lack of limit on size of parsed Range headers | `net/http@go1.27.1` | `go1.27.2` |
| 9 | GO-2026-6608 | Memory limit bypass when parsing MIME headers | `mime/multipart@go1.27.1` | `go1.27.2` |
| 10 | GO-2026-6607 | Reject malformed ECH outer extension references | `crypto/tls@go1.27.1` | `go1.27.2` |
| 11 | GO-2026-6605 | HTTP/1 client connection desynchronization after CONNECT rejection | `net/http@go1.27.1` | `go1.27.2` |
| 12 | GO-2026-6603 | HTTP/2 server memory exhaustion due to Trailer headers | `net/http/internal/http2@go1.27.1` | `go1.27.2` |

调用点分布在 `cmd/server`、`cmd/mcp-server`、`internal/httpapi`、`internal/qa`、`internal/llm`、`internal/qdrant`，逐条 trace 见原始输出文件。

### 3.2 不可达的 4 条

`govulncheck` 另报 1 条在 import 包中、3 条在 require 模块中，明确说明 `your code doesn't appear to call these vulnerabilities`。用 `-show verbose` 可展开；本轮未把不可达项计入"可达漏洞"，也不拿它当转绿理由。

### 3.3 处置登记

- **根因是工具链版本，不是业务依赖**：12 条全部由 `go1.27.1` 标准库引入，修复版本落在 `go1.27.2`～`go1.27.4`。
- **修复路径**：把本机 Go 升级到 ≥ `go1.27.4` 并重跑同一命令；升级后 `go.mod` 的 `go 1.27.1` 只是最低版本声明，是否需要同步提高由升级时决定。
- **本轮未执行升级**：升级 Go 是机器级变更，不在仓库内，且会让"同一命令同一环境复现"失去基线。因此按第 8 课口径**逐条登记、保留红**。
- `go vet ./...` 与 `go test ./...` 在 `go1.27.1` 下全绿（见 `node scripts/make-check.mjs security` 退出码 0），说明漏洞是标准库实现问题，不是本项目代码写错。

---

## 4. SEC-06 前端依赖审计【实测】

`npm audit --json` 退出码 1。元数据：

```json
{"info":0,"low":0,"moderate":1,"high":1,"critical":0,"total":2}
```

即 **2 个受影响包（1 moderate + 1 high）**，共 9 条 advisory 条目。

| 包 | 严重度 | 影响范围 | 是否直接依赖 | 修复建议 |
| --- | --- | --- | --- | --- |
| `esbuild` | moderate | `<=0.24.2` | 否（经 vite 引入） | 升 `vite@8.3.8`（major） |
| `vite` | **high** | `<=6.4.3` | 是 | 升 `vite@8.3.8`（major，`isSemVerMajor: true`） |

`vite` 的 8 条 advisory：

| GHSA | 严重度 | 标题 |
| --- | --- | --- |
| GHSA-587h-7wmj-h5pm | **high** | `fs.existsSync()` 存在性检查可在 Windows 被绕过 |
| GHSA-4w7w-66w2-5vf9 | moderate | 优化依赖 `.map` 处理的路径穿越 |
| GHSA-v6wh-96g9-6wx3 | moderate | Windows UNC 路径处理导致 NTLMv2 哈希泄露 |
| GHSA-698v-4r3x-3vcv | moderate | `escapeCmdArgs()` 在 Windows 漏掉 `%` 转义 |
| GHSA-r825-w8mm-5pxw | moderate | 编辑器 CLI 参数误读，某些编辑器下可任意命令执行 |
| GHSA-rq7h-c2jc-7f22 | moderate | 非 Windows 上项目路径匹配系统路径时可绕过 `server.fs.allow` |
| GHSA-fx2h-pf6j-xcff | moderate | Windows 备用路径绕过 `server.fs.deny` |
| GHSA-9jrq-w75r-8gcw | low | Vite 自定义中间件跨源脚本执行 |

`esbuild` 的 1 条：GHSA-67mh-4wv8-2f99（moderate）——任意网站可向开发服务器发请求并读取响应。

### 处置登记

- 2 条都只影响**开发服务器**，不进入生产镜像（生产是 Go 静态服务托管 `frontend/dist/`，见 `deploy/backend.Dockerfile`）。
- 官方修复是升到 `vite@8.3.8`，跨 2 个 major，且需连带确认 React 插件与构建产物兼容性。【未验证】未在本轮执行升级或回归，不写"已修复"。
- 因此**逐条登记、保留红**。

---

## 5. 判定

| 编号 | 判定 | 依据 |
| --- | --- | --- |
| SEC-05 | 绿 | 225 文件 0 命中，退出码 0，红绿证据齐 |
| SEC-06 | **红** | Go 可达 12 条（工具链）、npm 2 包（1 high）；已逐条登记，未清零 |

这是全表保留的那条红，与 `reports/security-precheck.md`、`reports/security-fix-log.md` 的判定一致。

---

## 6. 修复后复测（2026-11-21）【实测】

上面第 1～5 节保留为**原始红色基线**，不回改历史数字。本轮已把 `backend/go.mod` 与构建镜像升到 Go 1.27.4，把前端升到 Vite 8.3.8、`@vitejs/plugin-react` 6.3.0，并使用固定版本 `govulncheck@v1.10.0` 复测：

| 检查 | 修复前 | 修复后 | 证据 |
| --- | --- | --- | --- |
| Go 可达漏洞 | 12 条 | **0 条** | `reports/security-gate-latest.log` |
| npm 漏洞 | 2 包，含 1 high | **0 条** | 同上 |
| 前端生产构建 | 未验证 | 通过，23 modules | `npm run build` |
| SQL / XSS / CSRF 反例 | 分散脚本 | 常驻安全门禁，3/3 通过 | 同上 |
| 安全门禁故意失败 | — | 注入不可执行的审计命令后退出非零 | `reports/security-gate-intentional-red.log` |

统一复跑：`GO_BIN=/d/tools/go/bin/go.exe node scripts/make-check.mjs security`。govulncheck 仍提示所需模块中有 1 条当前代码不可达漏洞；本报告只把可达调用漏洞计为 0，没有隐藏该提示。原始基线中“保留红”是当时的处置，不再代表当前工作区状态。

本机没有 gcc，`go test -race` 因 `cgo: C compiler "gcc" not found` 未能执行；普通并发单测通过，不把竞态检测写成已验证。
