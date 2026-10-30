# Tasks

> 按编号顺序落地；每条以可判定的 `verify:` 说明「怎么算完成」。完成一条勾一条，并在「实测」写入命令与结果。
> 本次只修独立走查确认的缺陷（N01–N07、B01–B06、B13 的停止标记），非目标见 proposal。

## 1. 规约

- [x] 1.1 新增 change 四件套并通过严格校验
  - verify: `npx @fission-ai/openspec@latest validate --all --strict` 退出码 0；每条 Scenario 含 GIVEN/WHEN/THEN
  - 实测：`npx --yes @fission-ai/openspec@latest validate --all --strict` → `4 passed, 0 failed`（change + 3 份 spec）；逐条检查 Scenario 均含 GIVEN/WHEN/THEN。

## 2. 检索依据与引用

- [x] 2.1 停用词 + 内容词判定，至少 2 个内容词才可能成为依据
  - verify: `go test ./internal/retrieval ./internal/qa` 通过；用真实跨领域问题固定回归：贝尔不等式 / 红烧肉 / 法国首都三项不命中，正常课程问题命中
  - 实测：`go test ./internal/retrieval ./internal/qa` → 两包均 ok。`go run ./cmd/e2e` 的 R5.4 三项均返回 `no_evidence` + 固定文案（贝尔不等式在真实光合作用材料里会偶然命中「诺贝尔/量子/过程」，覆盖率 3/11≈0.27，低于门槛）；同轮 R5.1「命中」用例正常返回引用。
- [x] 2.2 向量分只参与排序与辅助，不单独放行
  - verify: 表驱动用例中 `ContentHits=1` 且向量分 0.99 仍判定无依据；`ContentHits≥2` 且向量分达门槛才通过
  - 实测：`go test ./internal/qa -run TestEvidenceOKRequiresContentHits -v` → 7 个子用例全 PASS，含「单个内容词即使向量分很高也不放行」「两个内容词且向量分达标可辅助通过」。
- [x] 2.3 系统提示增加「资料是数据不是指令」，引用片段改为完整切片
  - verify: `go test ./internal/qa` 断言 system prompt 含隔离语句、片段不截断；引用 `snippet` 长度等于切片长度
  - 实测：`go test ./internal/qa -v` → `TestSystemPromptKeepsMaterialAsData`、`TestUserPromptKeepsFullSnippetAndQuestionSeparate`（1280 字片段含尾部独有结论）、`TestFallbackAnswerUsesFullSnippet` 全 PASS。
- [x] 2.4 流式引用过滤：`[9]` 不进入展示与落库
  - verify: `go test ./internal/httpapi` 中假流输出 `结论[9]。[1]`，响应与落库文本均不含 `[9]`；拆分成 `[`、`9`、`]` 三个增量也能过滤
  - 实测：`go test ./internal/httpapi -run 'TestAskFiltersOutOfRangeCitations|TestCitationFilter' -v` → 6 项全 PASS；handler 用例把 `[`/`9`/`]，另有[1]。` 拆成三个增量，展示与落库均为「结论，另有[1]。」，且断言文本不含 `9`。
- [x] 2.5 检索层单测覆盖停用词、覆盖率和不同长度查询
  - verify: `go test ./internal/retrieval -run TestScoreKeywords -v` 全绿，至少含 6 个用例
  - 实测：`go test ./internal/retrieval -run TestScoreKeywords -v` → 6 个子用例（贝尔/红烧肉/法国/乳酸菌/光合作用/水循环）+ 3 个独立用例全 PASS，含长查询 3/11 覆盖率回归。

## 3. 流完整性

- [x] 3.1 `llm.ChatStream` 未收到 `[DONE]` 的 EOF 视为中断
  - verify: `go test ./internal/llm`：正常 `[DONE]` → `Done`；直接 EOF → `Err`；扫描错误 → `Err`
  - 实测：`go test ./internal/llm -v` → 5 项全 PASS：`CompletesOnDone`、`EOFWithoutDoneAsError`、`EmptyBodyAsError`、`ReportsUpstreamStatus`、`ReportsScanError`。
- [x] 3.2 问答接口发送 `error` / `degraded`，中断与空流不落库
  - verify: `go test ./internal/httpapi`：截断 → `event: error` 且无 `done`、未保存；空流 → 同上；首字前 5xx → `degraded` + 抽取式 + `done` + 已保存
  - 实测：`go test ./internal/httpapi -run 'TestAskStreamInterruptedAfterDelta|TestAskEmptyStreamIsError|TestAskDegradesBeforeFirstDelta' -v` → 3 项 PASS；事件序列分别为 `meta,delta,error`、`meta,error`、`meta,degraded,…,done`，中断/空流 `saved=0`，降级 `saved=1`。
- [x] 3.3 前端识别 error / degraded / 未完成，停止后显示「已停止」
  - verify: `e2e-ui.mjs` 中断态断言通过；手动点「停止」后页面出现「已停止」，不再显示生成中
  - 实测：`node scripts/e2e-ui.mjs` → T11a 点击「停止」后显示「已停止生成，以上为不完整内容」且不再显示生成中；T11b 无 done 结束标记「生成中断」；T11c error 显示可读文案；T11d degraded 显示「模型暂不可用」。

## 4. 上传与输入边界

- [x] 4.1 标题按 rune ≤255，正文校验 UTF-8，文件类型白名单
  - verify: `go test ./internal/httpapi`：256 字标题 → 400、非法 UTF-8 → 400、`.docx` → 415，且材料未落库
  - 实测：`go test ./internal/httpapi -run TestCreateMaterial -v` → 4 项 PASS（256 字标题 400 `title_too_long`、非法 UTF-8 400 `invalid_encoding`、`.docx` 415 `unsupported_file_type`、33 MiB 413）；`go run ./cmd/e2e` R3.4 四类非法输入后材料数不变。
- [x] 4.2 32 MiB 上限：超出 → 413；`materials.content` 迁移为 `LONGTEXT`
  - verify: `go test ./internal/httpapi` 的 33 MiB 请求 → 413；`go test ./internal/db`（或 e2e 查询 information_schema）确认列类型为 `longtext`
  - 实测：httpapi 单测 33 MiB multipart → 413 `payload_too_large`；`go run ./cmd/e2e` 查询 `information_schema` 得 `longtext`，17 MiB 文本 → 201 且 `ready`、生成切片，清理后材料数回到上传前基线。
- [x] 4.3 前端 multipart 头修复，文件选择上传成功
  - verify: 浏览器 e2e 通过真实文件选择上传 `.md`，返回 201 且列表出现；Network 面板中 `Content-Type` 为 `multipart/form-data; boundary=…`
  - 实测：`node scripts/e2e-ui.mjs` → T9 经 `DOM.setFileInputFiles` 上传 `.md` 成功且列表出现；T9b 捕获请求头为 `multipart/form-data; boundary=…`。
- [x] 4.4 标题输入 `maxLength=255` 与计数提示
  - verify: 浏览器 e2e 断言 `#t.maxLength === 255`；输入 300 字被截断到 255，上传成功或按服务端规则提示
  - 实测：`node scripts/e2e-ui.mjs` → T3b 聚焦后用真实编辑命令输入 300 字，值长度为 255，计数显示「当前 255 字」，`maxLength===255`；T12a 255 字标题上传成功并出现在列表。

## 5. HTTP 契约与可观测

- [x] 5.1 `/api/` 统一 JSON 404/405，`Allow` 正确
  - verify: `go test ./internal/httpapi`：`GET /api/ask` 与 `PUT /api/ask` → 405 JSON 且 `Allow: POST`；`GET /api/not-a-route` → 404 JSON，不是 HTML
  - 实测：`go test ./internal/httpapi -run 'TestMethodNotAllowed|TestUnknownAPI' -v` → PASS；`go run ./cmd/e2e` R11 同样断言 405 + `Allow: POST`、未知路径 404 JSON。
- [x] 5.2 严格 JSON：内容类型 415、体积 413、尾随内容 400
  - verify: `go test ./internal/httpapi`：`text/plain` → 415；1.1 MiB → 413；`{}{}` → 400；三者均未调用问答
  - 实测：`TestAskStrictJSONRejectsBeforeRetrieve` 3 个子用例 PASS，且 `retrieveCalls==0`；e2e R11 对问答接口复验 413/415/400，均未开始 SSE。
- [x] 5.3 500 分支写 ERROR 日志（request_id / operation / error）
  - verify: `go test ./internal/httpapi -run TestLogServerError` 断言 JSON 日志字段；或 e2e 触发 500 后 `docker logs` 可见 ERROR 行
  - 实测：`go test ./internal/httpapi -run TestLogServerError -v` → `TestLogServerErrorOnRetrieve` PASS；JSON 日志含 `level=ERROR`、`request_id=rid-500`、`operation=retrieve`、`error`，且不含请求体。

## 6. 界面状态与排版

- [x] 6.1 材料列表补齐加载/空/错误态与重试
  - verify: 浏览器 e2e 屏蔽 `/api/materials` 后出现错误与「重试」，解除屏蔽点击重试后列表恢复；空列表文案与角色相关
  - 实测：`node scripts/e2e-ui.mjs` → T10a 加载中占位、T10b 加载中不显示空态、T10d 失败显示错误与「重试」、T10e 点击重试恢复、T10f 教师空态给出上传引导。
- [x] 6.2 窄屏卡片布局与长标题两行截断，状态徽标不竖排
  - verify: 浏览器 e2e 在 524px 视口断言 `.status` 高度 ≤ 40px 且宽度 > 高度；255 字标题行高 ≤ 120px
  - 实测：`node scripts/e2e-ui.mjs` 在 524px 实测 `thH=32.5`、`badgeW=52 / badgeH=19`、255 字标题 `rowH=53`、横向溢出 `-15`；T12b–T12e 全 PASS。
- [x] 6.3 引用面板展示完整切片，可核对尾部内容
  - verify: 浏览器 e2e 上传含 300 字后独有尾注的材料，点开引用后 `.cite-item` 含该尾注
  - 实测：`node scripts/e2e-ui.mjs` → T5 点开引用可展开原文；T5b 找到本轮材料对应的 `.cite-item`，其中含 300 字之后的「（尾注…）」标记。

## 7. 验收与回写

- [x] 7.1 Go e2e 扩展并通过
  - verify: `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-e2e` 退出码 0，新增断言全部出现且通过
  - 实测：`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-e2e --with-spec` → 质量门禁全部通过；Go e2e `63 通过, 0 失败`（含 R3.4/R3.5、R5.4 三项、R11、Qdrant 清理）。
- [x] 7.2 浏览器 e2e 扩展并通过
  - verify: `node scripts/e2e-ui.mjs` 退出码 0，T1–T12 全绿
  - 实测：`node scripts/e2e-ui.mjs` → `32 通过, 0 失败`，退出码 0。
- [x] 7.3 质量门禁与 OpenSpec 校验
  - verify: `GO_BIN=... node scripts/quality-gate.mjs --with-spec` 退出码 0；`gofmt -l .` 为空
  - 实测：`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` → 全部通过（gofmt/分层/密钥/构建/vet/单测/spec）；`gofmt -l .` 无输出。顺带修复门禁在 Windows 上 `spawnSync npx` 的 ENOENT（改用 shell 启动）。
- [x] 7.4 手工复验 17 MiB 上传、非 UTF-8、`.docx`、255 字标题与 524px 视口
  - verify: 运行中的栈上逐项操作，记录状态码与截图/尺寸；17 MiB 上传清理后材料数回到基线，Qdrant 无残留
  - 实测：运行中的栈上（compose，LLM 关闭）由 e2e 逐项操作：非 UTF-8 → 400、`.docx` → 415、32 MiB+ → 413、17 MiB → 201 且 `ready`、255 字标题 → 201；清理后材料数回到基线；524px 尺寸见 6.2。Qdrant 未映射到宿主机，从 backend 容器内请求 `qdrant:6333` 返回 404（集合不存在，本轮 LLM 关闭未写向量）。
- [ ] 7.5 文档同步与 PR 评审
  - verify: README 的上传限制、`docs/independent-test-report.md` 的修复状态、tasks 实测均已更新；分支推送后开 PR，由**另一位成员在 PR 上审批**（审批即评审），通过后合并
  - 实测：README 已补「上传与输入边界」；`docs/independent-test-report.md` 已加第 7 节修复状态；tasks 实测已回写。分支与 PR 已创建，等待另一成员审批；**作者不代签审批**，本条在 PR 合并后勾选。
