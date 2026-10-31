# Tasks

> 按编号执行；每条任务在实测后勾选，并写入命令与结果。既有材料列表、上传、检索与问答断言不得为本变更让步。

## 1. 规约与接口

- [x] 1.1 建立 `material-reading` change 四件套
  - verify: `npx --yes @fission-ai/openspec@latest validate --all --strict` 退出码 0
  - 实测：`npx --yes @fission-ai/openspec@latest validate --all --strict` 输出 7 passed / 0 failed，其中包含 `change/2026-11-01-add-material-reader`，退出码 0。

- [x] 1.2 实现 `GET /api/materials/{id}` 与路由，跨班 404、非法 id 400、未认证 401
  - verify: `go test ./internal/httpapi` 通过；`go vet ./...` 通过
  - 实测：`go test ./internal/httpapi ./internal/store` 输出 `ok ... internal/httpapi 1.177s`；`go vet ./...` 与 `gofmt -l .` 均无输出、退出码 0。路由注册 `GET /api/materials/{id}`，非 GET 走 405；`TestGetMaterialRejectsInvalidID` 覆盖 `abc` / `0` / `-3`，均返回 400 `bad_request`。

## 2. 接口验收

- [x] 2.1 覆盖本班教师/学生读取与索引失败仍可读
  - verify: Go e2e 输出 MR1.1 与 MR1.4 断言全绿
  - 实测：加载 `deploy/.env` 并指定宿主机 MySQL 后执行 `go run ./cmd/e2e`，72 通过 / 0 失败。MR1.1 的「教师读取本班材料 → 200 且正文含独有词」「详情含索引状态与可解析的上传时间」「学生读取同一份本班材料 → 200 且正文一致」全部通过；MR1.4 的「failed 材料仍返回 200 与完整正文」通过。

- [x] 2.2 覆盖跨班、未认证与非法 id 负向
  - verify: Go e2e 输出 MR1.2 / MR1.3 断言全绿，跨班响应不含独有词
  - 实测：同一轮 Go e2e 中，MR1.3 的「未携带令牌读取详情 → 401」「非数字 id → 400 bad_request」「非正整数 id → 400 bad_request」通过；MR1.2 的「A 班教师读取 B 班材料详情 → 404 且不含 B 内容」「A 班学生读取 B 班材料详情 → 404 且不泄漏存在性」通过。

## 3. 阅读面板

- [x] 3.1 材料标题改为可聚焦阅读入口，新增加载/错误/空/正常四态面板
  - verify: `npm run build` 通过；真实浏览器可在本班材料与详情之间打开/关闭
  - 实测：`npm run build` 通过（tsc + vite，36 modules），产物进入 `backend/internal/web/dist`；`docker compose up -d --build backend` 重建生产容器后 `/health` 返回 200。浏览器 e2e 的 T3c（教师打开并读到完整正文）、T3f（Enter 打开）、T14b（学生打开）通过，四态由 T15a–T15e 覆盖。

- [x] 3.2 键盘、焦点回归与窄屏
  - verify: 浏览器 e2e MR2.4 通过；524px 无页面级横向溢出
  - 实测：T3e（Esc 关闭且焦点回到标题按钮）、T3g（打开后焦点落在关闭按钮）、T14d（学生关闭后焦点回归）通过；T15f 在 524px 实测阅读面板 `left=0`、`right=524`、页面横向溢出 `0`，内容区 `overflow-y: auto` 且文本无横向溢出。

- [x] 3.3 更新浏览器 e2e 与状态文档
  - verify: `node scripts/e2e-ui.mjs` 全部通过；`docs/project-status-guide.html` 的接口、状态与测试计数与实际一致
  - 实测：`BASE=http://127.0.0.1:8080 node scripts/e2e-ui.mjs` 最终为 50 通过 / 0 失败；`docs/project-status-guide.html` 已更新为 2026-11-01 快照，补充材料全文阅读、三个活动 change、Go e2e 72 项与浏览器 e2e 50 项，并保留「评审 / 提交 / 归档未闭环」的表述。

## 4. 全量回归

- [x] 4.1 重跑质量门禁与两套 e2e
  - verify: `GO_BIN=... node scripts/quality-gate.mjs --with-spec --with-e2e` 与 `node scripts/e2e-ui.mjs` 均退出码 0
  - 实测：`set -a; . deploy/.env; set +a; MYSQL_HOST=127.0.0.1 MYSQL_PORT=13306 GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec --with-e2e` 九项全部通过并输出「质量门禁全部通过。」，退出码 0（Go e2e 10267ms）。浏览器 e2e 50 通过 / 0 失败，退出码 0。未重启 MySQL / Qdrant，仅重建后端容器使新前端与详情接口生效。
