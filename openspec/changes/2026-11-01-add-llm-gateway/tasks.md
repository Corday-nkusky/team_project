# Tasks

> 按编号执行；每条在实测后勾选，并写入命令与结果。真实密钥只从本机 `deploy/.env` 注入，不得写入仓库、日志或验收记录。

## 1. 规约与配置

- [x] 1.1 建立 `llm-gateway` change 四件套
  - verify: `npx --yes @fission-ai/openspec@latest validate --all --strict` 退出码 0
  - 实测：`npx --yes @fission-ai/openspec@latest validate --all --strict` → `Totals: 8 passed, 0 failed (8 items)`，退出码 0；`quality-gate --with-spec` 复跑同样通过。

- [x] 1.2 启用时强制 base 与两个模型名，`/health` 暴露模型名
  - verify: `go test ./internal/config ./internal/httpapi` 与 `go vet ./...` 通过；缺 `LLM_CHAT_MODEL` / `LLM_EMBED_MODEL` 时启动失败，`/health` 启用时含两个模型名且不含 key/base
  - 实测：`quality-gate.mjs --with-spec` 的 go vet / go test 全通过。`cd deploy && docker compose run --rm --no-deps -e LLM_CHAT_MODEL= backend` 退出码 1，只输出 `LLM_API_KEY 已设置时必须同时提供 LLM_CHAT_MODEL`，未回显密钥。`GET /health` 返回 `{"deps":{"mysql":"ok","qdrant":"ok"},"llm":{"chat_model":"course-chat","embed_model":"course-embedding","enabled":true},"status":"ok",...}`，无 key 与 base。

- [x] 1.3 同步 `.env.example`、compose 与 README
  - verify: 样例不再提供模型名默认值；README 写明网关必须同时支持 `/chat/completions`（SSE）与 `/embeddings`，换嵌入模型需重建集合并重索引
  - 实测：`deploy/.env.example` 的 `LLM_API_BASE` / `LLM_CHAT_MODEL` / `LLM_EMBED_MODEL` 均为空占位，注释声明模型名无默认值；compose 透传四项而不写默认值。README 第 132–140 行写明对话需 SSE、嵌入返回维度需固定，更换嵌入模型或维度时先删除 Qdrant 集合 `team_campusclaw_chunks` 再重索引全部材料。

## 2. 真实网关验证

- [x] 2.1 新增真实网关客户端集成测试
  - verify: `LLM_API_KEY=... go test -tags=llm_integration ./internal/llm` 通过；未配置 key 时该测试跳过
  - 实测：未注入 key 时 `go test -tags=llm_integration ./internal/llm -v -run TestConfiguredGateway` → `LLM_API_KEY 为空，跳过真实网关集成测试`，SKIP。从本机 `deploy/.env` 注入后同一命令 PASS，日志 `嵌入维度=2048`、`流式回答长度=2`。

- [x] 2.2 启用本地栈完成上传到引用
  - verify: `/health` 的 `llm.enabled=true` 且模型名正确；新上传材料 `ready`；Qdrant 存在该材料向量点；学生提问收到增量、`done`、引用且无 `no_evidence`/`degraded`
  - 实测：`node scripts/verify-llm.mjs` 全部通过；材料 id 257 为 `ready`，问答收到 13 个增量与 3 条引用，回答与引用片段均含本轮独有验证词，无 `no_evidence` / `degraded` / `error`。从 backend 容器查询 Qdrant `points/count`（`material_id=257`）返回 `{"count":1}`。历史材料在启用前没有向量，需按 README 重索引后才会进入向量路。

- [x] 2.3 嵌入失败与重索引演练
  - verify: 临时把嵌入模型改为不存在值并重启后，上传材料为 `failed` 且 `index_error` 含嵌入失败；恢复配置并重索引后转 `ready`
  - 实测：将本机 `deploy/.env` 的 `LLM_EMBED_MODEL` 临时改为 `missing-model-for-drill` 并重启，上传 id 192 返回 201 但 `index_status=failed`，`index_error` 为 `生成嵌入失败: 模型返回 400: {...只允许这些模型：course-embedding}`。恢复原配置并重索引 id 192 后转为 `ready`；临时备份已删除，密钥未写入本记录。

## 3. 回归与收尾

- [x] 3.1 重跑质量门禁与 OpenSpec 严格校验
  - verify: `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` 输出「质量门禁全部通过。」且退出码 0
  - 实测：`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` → 配置样例、分层依赖、密钥与配置卫生、gofmt、go build、go vet、go test、OpenSpec 严格校验 8 项全部 ✓，输出「质量门禁全部通过。」，退出码 0。

- [x] 3.2 重跑两套 e2e
  - verify: Go 接口 e2e 与 `node scripts/e2e-ui.mjs` 均退出码 0；记录启用了真实网关这一环境事实
  - 实测：Go 接口 e2e 73 通过、0 失败（LLM 启用的本地栈；含 17 MiB 上传转 `ready` 与 Qdrant 旧向量清理断言）。`node scripts/e2e-ui.mjs` 51 通过、0 失败；运行环境 `/health` 为 `llm.enabled=true`、`chat_model=course-chat`、`embed_model=course-embedding`。浏览器键盘引用断言使用即时 mock 流：本机 headless Chrome 在真实长 SSE 结束后不再向页面派发 CDP 按键；真实长流路径继续覆盖引用内容与鼠标聚焦。

- [x] 3.3 更新状态文档并回填实测
  - verify: `docs/project-status-guide.html` 的 LLM 状态、验证计数与本 change 的实测一致；本文件所有 `[ ]` 已按实测勾选
  - 实测：已更新 `docs/project-status-guide.html`：`llm.enabled=true` 与模型名、73 / 73 与 51 / 51、全链路与失败演练结论、历史材料待重索引的限制，以及 change 已提交为 PR #4 的流程状态。本文件全部任务已按实测勾选。

## 4. 提交与评审

- [x] 4.1 提交 PR 并同步状态
  - verify: `gh pr view 4` 可读，base 为 `main`；状态文档写明 PR #3 已合并、PR #4 指向 main
  - 实测：2026-11-01 将本 change 提交并推送 `feat/llm-gateway` 并创建 PR #4；PR #3 于 2026-11-03 合入 `main`（`a39846e`）后，2026-11-06 将 PR #4 的 base 改为 `main`，同步了 main 内容并更新 `docs/project-status-guide.html` 的基线表述。PR 仍待真实评审与归档。
