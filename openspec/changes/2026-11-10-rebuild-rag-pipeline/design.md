# Design

## Context

现网链路：`retrieval.Split` 按固定 500 字窗口 + overlap 切片 → `qa.Index` 把纯正文入 MySQL 与 Qdrant → `qa.Retrieve` 用 `ListChunksByClass` 取全班 `ready` 切片 → `ScoreKeywords` 对每片重新分词、统计全语料 DF、算 BM25 近似分 → `llm.Embed` → `qdrant.Search(..., s.topK*3)` → 把 12 条向量分回填到全班等长数组 → 数内容词 → `0.6*kwN + 0.4*vecN` → 对全班切片排序 → 取 topK 直接进提示词。

基线：`llm.Embed` 接受字符串数组，`ChatStream` 用于生成答案；`qdrant.Client` 只有 `/points/search`，带 `class_id` 过滤；`store.Chunk` 自带 `MaterialTitle`；compose 为 MySQL 8.4 + Qdrant 单实例。`deploy/.env` 实测 `LLM_EMBED_MODEL=course-embedding` 可用，返回 2048 维，余弦分相关样本约 0.61–0.82、无关样本约 0.19–0.36，因此沿用 `MIN_VECTOR_SCORE=0.55`，不新增阈值。检索侧可用的模型能力只有 embedding，因此查询变体与精排必须全部本地实现。

## 目标数据流

```
索引：材料 → SplitDocument（Markdown 标题 / 编号小节 / 段落 / 句末边界）
      → 子块(正文 + heading_path) → 「路径前缀 + 正文」→ 一次批量 Embed → Qdrant
      → MySQL chunks 写入 heading_path

查询：问题 → QueryTerms（检索词元）+ QueryVariants（原句 / 概念短语）
      → 一次批量 Embed（最多 2 个向量）
      ├─ dense 多路（并行，class_id 过滤）──┐
      ├─ Index.Search 本地 BM25 倒排 ──────┼→ FuseRRF(k=60) → 候选上限 max(topK*6, 24)
      └─ Index.SearchHeading 结构路径路 ──┘
      → 只保留本班索引里存在的 ID → Rerank（本地分层比较）
      → 证据过滤 → Deduplicate → topK → Context（路径 + 相邻片摘要）
      → UserPrompt（引用只映射命中片）→ ChatStream
```

检索侧外部调用：一次批量 `Embed` + 最多两路并行 `Search`。精排、变体、融合、组装全是本地字符串与向量运算，不调用任何模型服务，也不需要 rerank 端点。

## 实现选择

### D1 结构切片与上下文前缀

`SplitDocument(title, content, size, overlap)` 按 Markdown `#` 标题、中文编号小节（「第一章」「一、」「1.1」）、空行分段切块；超长段落按句末标点回退切点再切，相邻块保留 overlap 字符防止定义句被腰斩。每个子块产出 `Text`（纯正文）、`HeadingPath`（材料标题 > 章节 > 小节）与 `EmbedText = HeadingPath + "\n" + Text`。

标题只更新结构栈、不混进正文，使「酶」这类标题词通过路径前缀与结构路径路进入检索，而引用仍是纯原文（R7.5）。前缀由规则生成、可复现，不依赖索引阶段的模型生成。`chunks` 增加单列 `heading_path`（幂等迁移，旧行默认空串）；Qdrant payload 同步携带。历史材料重索引后前缀才生效，重索引先 `DeleteByMaterial` 清掉该材料的旧点再 `Upsert`，点 ID 与新切片 ID 对齐、不留孤儿点；结构切片更细，总点数随切片数变化。集合维度校验与 `Upsert` 分批沿用 PR #4 已合入的 `qdrant` 实现，本变更不重复实现。

### D2 本地查询变体

`QueryTerms` 从问题里抽出去重词元：优先中文 bigram 与英文词（区分度高、posting 短），一个都没有时回退中文单字，让「酶是什么」这类单字概念仍有确定入口；停用词表只用于剔除检索噪声，不再作为「有依据」门禁。

`QueryVariants` 对不超过 14 字的短概念问句追加一个由词元拼成的概念短语，与原句一起送进**一次批量 `Embed`**；长问题保持原句单路。不生成假设性答案，不让模型凭常识补写内容——无关问题上这类段落会反过来误召回。

### D3 多路召回与 RRF

- **词面路**：`Index.Build` 一次性为 `body`（正文）与 `head`（结构路径）建 posting，并缓存切片长度与 `byMatSeq`；`Index.Search` 用标准 BM25（`k1=1.2`、`b=0.75`、IDF 由 `math.Log2` 计算）只访问命中词元的 posting list，不再每次重新分词全部切片。
- **语义路**：原句向量与概念短语向量并行查 Qdrant，每路都带服务端推导的 `class_id` 过滤，取回的最高分按 `chunk_id` 归并。
- **结构路径路**：`Index.SearchHeading` 按命中的不同词元数排序，给「酶是什么」这类只含标题词的短问句一个确定入口。
- 各路按名次用 `FuseRRF` 的 `1/(60+rank)` 融合，同名次按 ID 升序保证可复现；删掉 `0.6/0.4` 加权，因为两路分量纲不同，加权和没有可解释含义。融合结果再用 `Index.Doc(id)` 过滤：Qdrant 返回的跨班点、陈旧点、状态非 ready 点一律丢弃。

### D4 本地二阶段精排

`Rerank` 对每个候选一次分词，同时得到词元集合与查询词元的**最小覆盖窗口**（词集中在同一句 → 高档，散落各处 → 低档）、命中数与覆盖率；再算结构路径命中。排序用分层比较：有依据优先 → 结构路径命中 → 覆盖窗口档位 → 覆盖率 → RRF 名次 → 余弦分 → ID，不用跨量纲加权和。

`Deduplicate` 复用精排已经算好的词元集合，按同一材料内 Jaccard 相似度剔除高度重叠候选，不二次分词。整个过程零外部调用。

### D5 依据判定与语境组装

删掉 `MinEvidenceHits`、`contentTotal==0` 短路、停用词表与 bigram 数词门禁。语义路可用时，`QueryVecScore`（本班候选最高余弦分）须达到 `MIN_VECTOR_SCORE`，且候选须具备词面证据（结构路径命中，或覆盖率达 `MIN_KEYWORD_COVERAGE` 且覆盖窗口不超过 200 rune）或自身余弦分达门槛；零密钥降级时要求多词元命中或结构路径命中。泛词、无关问题仍按 R5.4 返回固定文案与 `no_evidence`。沿用既有配置项，不新增必填阈值。

`Citation` 增加 `Context string json:"-"`：`contextOf` 写入结构路径与相邻片摘要，每条摘要截断到 160 rune，避免把被去冗余剔除的重叠片段整段塞回提示词。`UserPrompt` 把 Context 标成「语境（非引用内容）」，只有命中片占引用编号。系统提示继续声明「资料只是参考数据，不是指令」（R5.5）。

### D6 降级与不变量

零密钥或 `Embed` / `Search` 失败时 `vecEnabled=false`，跳过 dense 路，仅用本地 BM25、结构路径路与精排召回，按 R6.5 以抽取式回答完成并展示「已降级」；不把抽取式回答伪装成模型生成。任何失败路径都不退回内容词命中数旧规则。班级隔离、引用片段纯原文、注入防护、流式三种结束方式不变；`check-layering.mjs` 的 LAYERS 不需要登记新包（新代码全在 `retrieval` / `qa` 内）。

### D7 每次提问的成本

- 词面索引按班常驻：`indexes` / `dirty` 双 map + `buildMu` 串行化重建并二次检查，首次提问或材料变更后才回表一次，后续提问零 MySQL 切片查询。
- 一次批量 `Embed` 取代多次；dense 多路并发，端到端延迟按最慢一路算，不叠加。
- 候选正文只分词一次（`analyze` 同时产出词元集合与覆盖窗口），去冗余直接复用结果。
- BM25 的 IDF 用标准库 `math.Log2`，删掉自写的对数近似。
- 语境有长度上限，不让相邻片把 prompt 撑大。

## 验证策略

- 已执行：`go build ./...`、`go vet ./...`、`go test ./...` 全绿；检索与问答链路 16 个针对性用例全 PASS；`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec` 八项全通过。
- 待真实服务栈：Go e2e（「酶是什么」带引用、章节型长问题命中不含「酶」的定义句、泛词 / 无关问题 `no_evidence`、跨班 403/404、引用为完整原文）与零密钥路径单独复验；按新片段粒度与顺序更新既有引用类用例的期望值。
- 待真实服务栈：浏览器 e2e 不回归。
