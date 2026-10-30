# Design

## Context

- 独立走查报告 `docs/independent-test-report.md` 给出 N01–N07 与 B 系列确认结果；本次只修其中已确认、且不需要新功能的部分。
- 现有检索把每个相邻汉字组合都视为「有信息量」，用「命中数 > 0 且覆盖率 ≥ 0.25」判定有依据；泛词即可击穿（N01）。
- 现有 SSE 只有 `meta / delta / done`；上游异常、空流、截断都落到 `done`，前端无法区分完整与半截（B02）。
- 上传上限对外按 32 MiB 设计，但 `materials.content` 是 `MEDIUMTEXT`（16 MiB），且只有 `TrimSpace`，没有编码、标题长度、文件类型校验（N02 / B03）。
- 前端 `request()` 对所有带 body 的请求设置 `Content-Type: application/json`，覆盖了浏览器为 FormData 生成的 multipart 边界（B01）。
- 规则约束：先写 change；失败路径与可判定 `verify:` 必备；不弱化断言；审计只增；密钥不外泄；迁移向前兼容。

## Goals / Non-Goals

**Goals:**

- 让「无依据」判定不再被泛词击穿，同时保留正常短问题的召回。
- 让流式回答的「完成 / 中断 / 降级 / 用户停止」在协议与界面上都可区分。
- 让上传边界与数据库容量一致，超限、错类型、错编码都返回明确状态码且不落库。
- 让 HTTP 契约与错误日志可判定：404/405/413/415、尾随 JSON 400、500 有 ERROR 日志。
- 用单元测试 + Go e2e + 浏览器 e2e 三层回归锁住上述行为。

**Non-Goals:**

- 不新增材料详情/删除、问答历史/多轮、异步索引、限流、Markdown 渲染（见 proposal 的非目标）。
- 不引入新的检索依赖（倒排索引、向量库重排）；仍以现有内存 BM25 近似 + 可选向量融合为基础。
- 不改变班级隔离、令牌、审计只增等既有约束。
- 不重写数据库 schema 工具；只做一次幂等的列扩容。

## Decisions

### D1 依据判定：停用词 + 内容词 + 至少 2 个内容词命中

`retrieval.tokenize` 仍生成中文 unigram（弱）与相邻 bigram（强）、ASCII 词（长度 ≥2 强）。在此基础上新增「内容词」判定：
- 中文 bigram 命中停用表，或含有功能字（`的 了 是 在 和 与 或 及 吗 呢 吧 啊 呀 哦 么 什 哪`）时，不算内容词；
- ASCII 常用功能词（`the/is/of/and/to/what/how/why/where/which/can/you/we` 等）不算内容词；
- 其余强词为内容词。

`ScoreKeywords` 返回每条文档的 `ContentHits`（命中的唯一内容词数）与 `ContentTotal`（查询中的唯一内容词数），`Coverage = ContentHits / ContentTotal`。`qa.Retrieve` 的准入条件改为：

```
ContentHits >= 2 && (Coverage >= MIN_KEYWORD_COVERAGE || VectorScore >= MIN_VECTOR_SCORE)
```

`MIN_KEYWORD_COVERAGE` 默认取 **0.30**（原 0.25）。向量分只作为「已有内容词证据」之上的补充，不再单独放行。这样：
- 「如何在家做红烧肉」只剩 `家做/做红/红烧/烧肉`，命中不足 2 个 → 拒答；
- 「法国的首都是哪里」只剩 `法国/首都`，即使两者都命中也是 2/2，仍可能通过——因此停用表还把 `都是/哪里` 剔除，且实际材料通常不同时含「法国」「首都」；回归用例用真实跨领域问题固定该行为；
- 「贝尔不等式的量子力学推导过程是什么」在真实材料里会偶然命中 `贝尔`（来自「诺贝尔」）、`量子`、`过程` 三个子串；3/11≈0.27，低于 0.30 → 拒答。单靠「至少 2 个内容词」挡不住这类长查询；
- 正常问题「乳酸菌的最佳繁殖温度是多少」可命中 `乳酸/酸菌/最佳/繁殖/温度` 等 ≥2 个 → 通过；线上真实问句「教师应该在哪里上传讲义」覆盖率为 4/7≈0.57，与门槛有明显间距。

被放弃的候选方案：
- **只加停用词、仍保留覆盖率 0.25**：停用表永远不全，阈值仍会被下一个泛词击穿。
- **IDF 加权覆盖率**：更优雅，但需要稳定语料与调参；本轮先用「至少 2 个内容词」这一可解释的硬约束，后续有标注集再升级。
- **向量分单独放行**：最近邻总会返回非零分，正是 N01 被放大的原因。

### D2 引用编号在服务端过滤，且流式安全

模型输出按增量到达，`[9]` 可能被拆成 `[`、`9`、`]` 三个事件。新增 `citationFilter`：
- 遇到 `[` 后最多缓冲 4 个字符等待 `]`；
- 完整 `[n]` 且 `1 ≤ n ≤ len(citations)` → 原样输出；
- 越界或非正数 → 丢弃并计数，流结束写一条 `slog.Warn`（只记数量，不记回答内容）；
- 不是数字形态的 `[...]`（如 `[注]`）原样输出，避免误伤正文。

过滤同时作用于模型流与抽取式降级文本，最终落库的是过滤后的文本，保证「展示 = 落库」。

引用片段改为完整切片（≤ `CHUNK_SIZE` 个字符，默认 500），不再截到 300 字；提示词也使用完整片段。这样点击引用看到的就是该切片原文，而不是再次截断的摘要（B04）。

被放弃的候选方案：
- **生成结束后整体正则替换**：无法解决流式过程中已经发出去的 `[9]`，用户仍会短暂看到假角标。
- **前端隐藏越界角标**：落库文本仍带 `[9]`，导出/复盘时依旧指向不存在的来源。

### D3 SSE 增加 `error` 与 `degraded`，中断不落库

协议：

| 事件 | data | 语义 |
| --- | --- | --- |
| `meta` | `{citations, no_evidence}` | 检索结果与引用清单 |
| `degraded` | `{reason}` | 模型在首字前不可用，后续为抽取式回答 |
| `delta` | `{text}` | 增量文本 |
| `error` | `{code, message}` | 生成中断，连接随后关闭，**不发送 done** |
| `done` | `{conversation_id}` | 完整结束 |

`llm.ChatStream` 在扫描结束时若未收到 `[DONE]`，发送 `Event{Err: io.ErrUnexpectedEOF}`；扫描错误原样发送。`handleAsk`：
- 上游错误且已有增量 → 发 `error`，审计 `ask fail 生成中断`，不落库；
- 上游错误且尚无增量 → 发 `degraded`，走抽取式回答并正常 `done`；
- 上游只发 `[DONE]` 而无任何增量 → 视为空回答，发 `error`，不落库；
- 客户端断开 → 不发事件、不落库，审计 `客户端中断`（保持现状）。

为了能对上述分支做无数据库单元测试，`Server` 的 `qa` 字段改为窄接口（`Retrieve / LLMEnabled / ChatStream / Index`），问答落库与审计提取为 `askRecorder` 窄接口；生产装配仍注入 `*qa.Service` 与 `*store.Store`。

前端 `streamAsk` 增加 `onDegraded`，并把「流结束但未见 done/error」判定为中断；`AskPage` 显示「已停止 / 生成中断 / 已降级」状态，半截回答不再看起来像完整回答。

被放弃的候选方案：
- **用 HTTP 状态码表达流中错误**：SSE 已发送 200，无法再改状态码。
- **中断后补发 done 并带 `incomplete: true`**：客户端与既有消费者都只认 done，容易再次把半截当完整；独立 `error` 更明确。

### D4 上传边界：32 MiB 与 LONGTEXT 对齐，先校验后落库

- `maxUploadBytes = 32 << 20`；JSON 上传请求体上限 `32 MiB + 1 MiB`（字段与转义开销），multipart 总上限 `32 MiB + 1 MiB`，文件本身上限 32 MiB。
- 标题：去空白后按 rune ≤255（`VARCHAR(255)` 在 utf8mb4 下按字符计）；超限 400 `title_too_long`；为空回退「未命名材料」。
- 正文：multipart 文本文件先 `utf8.Valid`，非法 400 `invalid_encoding`；PDF 抽取后为空 400 `empty_content`（提示扫描件需 OCR）。
- 类型：仅 `.txt / .md / .markdown / .pdf`，其余 415 `unsupported_file_type`；JSON 上传不接受文件类型字段。
- 体积：`http.MaxBytesReader` + `io.LimitReader` 双保险；超限 413 `payload_too_large`，两种提交方式一致。
- 数据库：`materials.content` 改为 `LONGTEXT`。`Migrate` 在 `CREATE TABLE IF NOT EXISTS` 之后查询 `information_schema.COLUMNS`，发现不是 `longtext` 才执行 `ALTER TABLE materials MODIFY COLUMN content LONGTEXT NOT NULL`；幂等、向前兼容、不丢数据。
- 前端：标题输入 `maxLength=255` 并显示计数；文件选择框 `accept` 与后端白名单一致。

被放弃的候选方案：
- **把上限降到 16 MiB 迁就 MEDIUMTEXT**：对外已按 32 MiB 设计与文档化，降级会让已上传的大材料受限；扩列更直接。
- **只依赖 `ParseMultipartForm(32<<20)`**：该参数是内存阈值，不是请求硬上限；不包 `MaxBytesReader` 仍可被超大请求打满内存。

### D5 严格 JSON 与统一 404/405

`decodeJSON` 统一：
1. 要求 `Content-Type` 为 `application/json`（允许 `; charset=...`），否则 415 `unsupported_media_type`；
2. `http.MaxBytesReader` 限制 1 MiB（上传走专用上限），`*http.MaxBytesError` → 413 `payload_too_large`；
3. 解码一个值后调用 `dec.Token()`，必须得到 `io.EOF`，否则 400 `bad_request`（尾随内容）。

路由：
- 为每个已注册路径追加一个「无方法」处理器；Go 1.22 的方法专用模式优先匹配，未支持方法落到该处理器，返回 JSON 405 并写正确 `Allow`（如 `/api/ask` 只允许 POST）。
- 注册 `/api/` 与 `/api` 兜底，返回 JSON 404，不再回落到 SPA HTML。
- SPA 兜底仍只处理 `GET /` 的非 API 路径。

被放弃的候选方案：
- **自定义 middleware 检查 method**：需要维护路径表，容易与路由漂移；利用 ServeMux 的优先级更稳。
- **`DisallowUnknownFields`**：可能打断带了额外字段的旧客户端；本轮只修「尾随内容」这一确定性问题。

### D6 错误日志：只加在 500 分支，字段固定

新增 `logServerError(r, operation, err)`：`slog.ErrorContext`，字段 `request_id`、`operation`、`error`。只用于确实返回 500 的分支（落库失败、查询失败、检索失败、保存会话失败）；4xx 不写 ERROR，避免把用户输入错误当故障。不记录请求体、令牌、SQL 原文。

被放弃的候选方案：
- **在访问日志中间件统一记录所有 5xx**：拿不到底层 error，只能记状态码，等于没加。
- **把 error 直接返回给客户端**：可能泄漏 SQL/内部细节。

## Risks / Trade-offs

- **召回下降**：要求 2 个内容词会拒掉一些极短或高度口语化的问题（如只问「酶」）。这是有意的保守取舍；先用回归集确认正常课程问题仍能命中，后续可用标注集调参。
- **停用表维护**：停用表是人工列表，可能漏词。用「功能字 + 常用词」两层覆盖，并把本轮三条跨领域问题固化为回归用例；新增误判时先补用例再补词。
- **SSE 协议扩展**：老客户端只认 `meta/delta/done`，遇到 `error` 会继续等到流结束；本仓前端同步升级，外部消费者需按新协议处理。
- **`ALTER TABLE` 锁表**：`LONGTEXT` 扩容在 MySQL 8 通常为 INSTANT/INPLACE，但大表仍可能短暂锁表；本地与课程环境数据量小，发布前按运维规范做快照。
- **33 MiB 拒绝测试**：e2e 会构造大请求，耗时与内存略增；只发一次，超限即断，不解析完整表单。

## Migration Plan

1. 启动时 `Migrate` 自动把 `materials.content` 扩为 `LONGTEXT`（幂等）。
2. 代码先兼容两种列类型（读取不依赖类型），再发布；回滚只需回退镜像，列扩容不阻塞旧版本。
3. 无数据回填；切片与索引不变。

## Test Strategy

| 层 | 覆盖 |
| --- | --- |
| `go test ./internal/retrieval` | 三条跨领域问题不命中；正常课程问题命中；停用词不计入内容词 |
| `go test ./internal/qa` | 提示词含注入隔离；完整片段进入引用与提示词 |
| `go test ./internal/llm` | 无 `[DONE]` 的 EOF → Err；正常 `[DONE]` → Done |
| `go test ./internal/httpapi` | 越界引用过滤、SSE error/degraded/不落库、严格 JSON、404/405、上传 400/413/415、错误日志字段 |
| `cmd/e2e` | 真实 MySQL/Qdrant/HTTP：N01 回归、上传边界、405/413/415、尾随 JSON、列类型 |
| `e2e-ui.mjs` | 真实浏览器：文件选择上传、引用完整片段、错误态重试、窄屏状态徽标、标题上限 |
