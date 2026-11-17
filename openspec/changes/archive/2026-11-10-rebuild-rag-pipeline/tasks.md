# Tasks

> 实现任务已落地并写入实测。后端栈已通过 `启动项目.cmd` 启动（MySQL / Qdrant / backend 均 healthy）；
> 尚需真实服务的验收项单列在「6. 验证」，因 embedding 网关当日额度耗尽未能跑完，一律标为**未验证**，不写成通过。
> 班级隔离、拒答、引用、降级与流式断言不为本变更让步。

## 1. 基线

- [x] 1.1 核对 embedding 网关能力
  - verify: `llm.Embed` 接受 `[]string`、`qdrant.Client.Search` 带 `class_id` 过滤
  - 实测：当前工作区 `qa.go` 调用 `s.llm.Embed(ctx, embedTexts)` 与 `s.vec.Search(ctx, Collection, queryVec, classID, limit)`，`go build ./...` 通过；`deploy/.env` 的 `course-embedding` 实测返回 HTTP 200 / 2048 维。按用户指示，PR #4 视为已通过，未另查 GitHub 评审状态。
- [x] 1.2 建立 change 四件套并通过 OpenSpec 严格校验
  - verify: `npx --yes @fission-ai/openspec@latest validate --all --strict` 退出码 0
  - 实测：`Totals: 6 passed, 0 failed (6 items)`，含 `✓ change/2026-11-10-rebuild-rag-pipeline`，退出码 0。
- [x] 1.3 主规约待同步项
  - verify: 旧 R5「内容词不少于 2 个」条款与对照表 `retrieval-evaluation` 残留均已处理
  - 实测：`openspec/specs/knowledge-retrieval/spec.md` 的 R5 已替换为结构切片 / 多路召回 / 本地精排，新增 R5.6–R5.9；`knowledge-retrieval` 与 `auth-upload` 两份主规约对照表已删除 `retrieval-evaluation` 行并把 `knowledge-retrieval` 场景范围更新为 R5.1–R5.9。

## 2. 索引侧

- [x] 2.1 `SplitDocument` 按标题层级 / 段落 / 句末边界结构切片，产出 `Text` / `HeadingPath` / `EmbedText`
  - verify: 超长段落按句子边界再切；标题不混入正文；切分结果可复现
  - 实测：`go test ./internal/retrieval -count=1 -run 'TestSplitDocument' -v` → `TestSplitDocumentKeepsHeadingOutOfSnippet`、`TestSplitDocumentSplitsLongParagraphAtSentenceBoundary` 全 PASS。
- [x] 2.2 `chunks` 增 `heading_path` 单列并做幂等迁移
  - verify: 复用 `db.Migrate` 的 information_schema 查列模式，重复执行不报错
  - 实测：`db.Migrate` 追加 `migrateChunkHeadingPath`，建表 DDL 与迁移均带 `NOT NULL DEFAULT ''`；`go build ./...` / `go vet ./...` 通过。真实 MySQL 8.4 上容器多次重启均正常执行，`SHOW COLUMNS FROM chunks LIKE 'heading_path'` 返回 `varchar(500) NOT NULL`，重复执行不报错。
- [x] 2.3 `qa.Index` 用「材料标题 > 章节路径 + 正文」一次批量嵌入，payload 带 `heading_path`
  - verify: `chunks` 正文与 `Citation.Snippet` 仍是纯原文，不含前缀（R7.5）
  - 实测：`ReplaceChunks` 接收 `[]store.ChunkSeed`，`EmbedText` 只送 embedding，`Text` 只入库；`go test ./internal/qa -count=1 -run 'TestUserPrompt|TestContext|TestFallback' -v` → 3 个用例全 PASS，确认完整 snippet 与 Context 不进引用清单。
- [x] 2.4 向量层沿用 PR #4，不重复实现
  - verify: `qdrant.go`、`config.go`、`llm.go` 与 PR #4 逐字一致，`qa.go` 只调用既有方法
  - 实测：融合后四个文件与 `origin/feat/llm-gateway` 逐字相同；`go build ./...` / `go vet ./...` / `go test ./...` 全绿，含 PR #4 新增的 `qdrant_test.go` / `llm_test.go` / `config_test.go`。维度不符分支由 PR #4 实现覆盖，未在本机触发。
- [x] 2.5 历史材料重索引
  - verify: 复用 `DeleteByMaterial` + `Upsert`，切片与向量 ID 对齐、payload 带 `heading_path`
  - 实测：新增 `scripts/reindex-all.mjs`（登录教师账号逐篇调 `/api/materials/{id}/reindex`，幂等、断点续跑、遇 429 立即停，进度存 `.workbuddy/reindex-progress.json`）。试点 id=1 由 7 个固定窗口切片变为 19 个结构切片且 `heading_path` 全有值，Qdrant 点 ID 换成新切片 ID、payload 带路径。全量执行后 56/98 篇重建成功；`course-embedding` 在 id=57 返回 429「今日 token 额度已用完」，脚本按设计停止，剩余 42 篇待额度恢复后重跑同一命令续完。
  - 重建前基线：98 篇材料中仅 25 篇 `ready`（旧链路向量）、73 篇 `failed`（同为 429）；614 个切片 `heading_path` 全为空串；Qdrant 171 点中 11 个是孤儿 ID、payload 无 `heading_path`。重建后：56 `ready` / 42 `failed`，1136 个切片中 885 个带结构路径，Qdrant 859 点 / 2048 维。
  - 已知边界：`qa.Index` 先写切片再调 embedding，embedding 失败时材料被置 `failed` 但切片已换成新格式，Qdrant 仍留旧点；`Retrieve` 用 `idx.Doc(id)` 过滤陈旧点，不会误引用。id=57 处于该状态，进度文件未记为完成，续跑会覆盖修复。

## 3. 召回

- [x] 3.1 自研 BM25 倒排索引（`index.go`），进程内按班常驻
  - verify: 查询只访问命中词元的 posting list；不对全部切片重新分词或统计全语料 DF
  - 实测：`go test ./internal/retrieval -count=1 -run 'TestInvertedIndex|TestIndexNeighbours' -v` → 2 个用例全 PASS；`buildMu` 串行化重建并在锁内二次检查，命中缓存不回表。
- [x] 3.2 本地查询变体：短概念问句追加概念短语，与原句合成一次批量 `Embed`
  - verify: 长问题保持单路；检索侧无额外生成式调用、无假设性答案
  - 实测：`go test ./internal/retrieval -count=1 -run 'TestQueryTerms|TestQueryVariants' -v` → 3 个用例全 PASS；`QueryVariants` 最多产出 2 个输入，`Retrieve` 只调用一次 `Embed`。
- [x] 3.3 dense 多路并行查询 Qdrant
  - verify: 每路带 `class_id` 过滤，并行发出，不串行叠加延迟
  - 实测：`go build ./...` / `go vet ./...` 通过；并发 goroutine + `WaitGroup` + 带缓冲 channel 已在代码中，真实 Qdrant 上的问答请求已返回 200。并行延迟未做仪器化观测，标为代码级确认。
- [x] 3.4 结构路径命中路 + RRF（`k=60`）依名次融合
  - verify: 删掉 `0.6/0.4` 加权与 `norm`；融合只由名次计算；跨班/陈旧点被过滤
  - 实测：`go test ./internal/retrieval -count=1 -run 'TestHeadingRoute|TestFuseRRF' -v` → 2 个用例全 PASS；融合后只用 `idx.Doc(id)` 中存在的 ID 建候选。
- [x] 3.5 `qa.Retrieve` 删除每次提问的 `ListChunksByClass` 全表拉取与全语料打分
  - verify: 命中索引缓存的路径不执行读取全班切片的查询
  - 实测：`go build ./...` 通过；`classIndex` 缓存命中时直接返回 `*retrieval.Index`，`ListChunksByClass` 只在重建时调用。SQL 计数未做仪器化观测，标为代码级确认。

## 4. 精排与判定

- [x] 4.1 本地二阶段精排：最小覆盖窗口、结构路径命中、覆盖率、分层比较排序
  - verify: 全程字符串与向量运算，零外部调用；不用跨量纲加权和
  - 实测：`go test ./internal/retrieval -count=1 -run 'TestMinCoverWindow|TestRerank' -v` → 3 个用例全 PASS；`analyze` 一次分词同时产出词元集合与覆盖窗口。
- [x] 4.2 按边际相关性剔除高度重叠候选，去冗余复用精排词元集合
  - verify: 同一段的多份切片不同时进入引用清单；不二次全文分词
  - 实测：`go test ./internal/retrieval -count=1 -run TestDeduplicate -v` → PASS；`Deduplicate` 直接读 `Candidate.terms`。
- [x] 4.3 依据判定改为查询级向量分 + 片段级证据，删掉 `MinEvidenceHits` / `contentTotal` 短路 / bigram 数词门禁
  - verify: 沿用 `MIN_VECTOR_SCORE` / `MIN_KEYWORD_COVERAGE`；泛词与无关问题仍 `no_evidence`
  - 实测：`go test ./internal/retrieval -count=1 -run 'TestRerankAdmitsShortConceptAndRejectsCrossDomain|TestRerankZeroKeyStillAnswersDoubleCharConcept|TestQueryTermsRejectsPureFiller' -v` → 全 PASS；`config.go` 未新增配置项。

## 5. 组装与降级

- [x] 5.1 命中片为核心，`Context`（结构路径 + 相邻片摘要）只进提示词，摘要截断到 160 rune
  - verify: 只有命中片进入引用清单；引用片段仍是完整原文；系统提示保留注入防护声明（R5.5）
  - 实测：`go test ./internal/qa -count=1 -run TestContextGoesToPromptButNotCitationList -v` → PASS，`json.Marshal(cites)` 不含「章节路径」「相邻片段」，引用条目数不增加。
- [x] 5.2 零密钥 / 网关故障走本地 BM25 + 结构路径路 + 精排 + 抽取式回答
  - verify: `degraded` 标记可见；任何失败路径不退回内容词命中数旧规则
  - 实测：单测 `TestRerankZeroKeyStillAnswersDoubleCharConcept`、`TestFallbackAnswerUsesFullSnippet` 全 PASS。真实 SSE 降级流未复验，原因见 6.2。

## 6. 验证与归档

- [x] 6.1 静态门禁
  - verify: `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` 退出码 0
  - 实测：配置样例、分层依赖、密钥与配置卫生、gofmt、go build、go vet、go test、OpenSpec 严格校验 8 项全 ✓，输出「质量门禁全部通过。」，退出码 0。另有 `go test ./...` 全包 ok（`httpapi 1.749s`、`qa 0.656s`、`retrieval 0.450s`）。
- [ ] 6.2 Go e2e（引用类用例按新片段粒度与顺序更新期望值）
  - verify: 「酶是什么」带引用、章节型长问题命中不含「酶」的定义句、泛词 / 无关问题 `no_evidence`、跨班 403/404、引用为完整原文
  - 实测：**未验证**。`curl http://127.0.0.1:8080/health` 返回 `HTTP=200`，MySQL / Qdrant / backend 三容器 healthy；但 e2e 会上传新材料并触发 embedding，而 `course-embedding` 当日额度已耗尽（单条探测返回 429），无法完整跑完。额度恢复、补齐 2.5 的 42 篇后再跑。
- [ ] 6.3 零密钥路径与浏览器 e2e 不回归
  - verify: `scripts/e2e-ui.mjs` 与零密钥 SSE 流退出码 0
  - 实测：**未验证**，原因同 6.2。
- [ ] 6.4 同伴评审签字（AGENTS.md 第八节，自审不算）
  - verify: `docs/lesson-07/peer-review.md` 签署
  - 实测：**待人工**，不代签。

## 未改动的文件

工作区中与本变更无关的个人笔记和文档改动不在提交范围内；它们不影响编译、门禁与运行。
