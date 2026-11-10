# CampusClaw 队友部署与运行说明

> 给队友及其编码 Agent 的执行文档。按顺序执行；每一步都有期望结果。遇到不符合的情况先停下来记录，不要猜。
>
> 当前快照：2026-11-10。PR #1–#5 已全部合并进 `main`，`main` 就是可用的稳定基线。
>
> 分支现状：
>
> - `main`：工作台、材料全文阅读、可配置 LLM 网关、检索链路重写（`scripts/verify-llm.mjs` 已在 `main` 上）；
> - `feat/rebuild-rag-pipeline`：在 `main` 之上多了检索链路重写提交与一键启动脚本（`启动项目.cmd` / `scripts/launch.mjs` / `scripts/reindex-all.mjs`）。
>
> 两条分支的启动方式完全相同，差别只在功能多少。**不要凭本文猜分支内容**，用这两条命令确认：
>
> ```bash
> git log --oneline -5
> git log --oneline main..origin/feat/rebuild-rag-pipeline
> ```

## 0. 目标与边界

目标：在一台全新机器上，从 clone 开始，把 CampusClaw 跑起来，完成登录、上传材料、提问与引用验证；需要时接入课程网站提供的模型 API。

不在本文范围：公网部署、生产运维、修改业务代码、压测。

安全边界：

- 真实的 `LLM_API_KEY` 只放在本机 `deploy/.env`，不提交、不截图、不发群、不写进任何文档。
- 不要复制别人的 `deploy/.env`。每个人用自己的课程 API key。
- 上传的材料会发送到 `deploy/.env` 中声明的模型网关；不要用真实敏感资料测试。
- 仓库里没有公网部署地址，本文只覆盖本机 Docker Compose。

### 最短路径（命令版）

```bash
git clone https://github.com/Corday-nkusky/team_project.git
cd team_project

# 想用双击脚本和最新检索链路：
git switch feat/rebuild-rag-pipeline
# 或只要稳定基线：保持在默认的 main

node scripts/up.mjs
curl -sS --noproxy '*' http://127.0.0.1:8080/health

# 按第 7 节把四项 LLM 配置写入 deploy/.env，然后：
cd deploy && docker compose up -d
cd .. && node scripts/verify-llm.mjs
```

不接模型也能跑完：跳过 `LLM_*` 配置，问答退化为抽取式回答，检索只走词面路。

## 1. Agent 执行约定

如果本文是给编码 Agent 执行的，遵守以下规则：

1. 按章节顺序执行，不跳步；每一步记录命令、退出码和关键输出。
2. 只修改本机 `deploy/.env`，不修改仓库里的业务代码、配置样例或文档。
3. 不执行 `git commit`、`git push`、`docker compose down -v`，除非人类明确要求。
4. 不要在终端回显 `LLM_API_KEY`；需要展示 `.env` 时，先把值遮住。
5. 首次构建可能超过 10 分钟，不要因为暂时没有输出就中断。
6. 任何一步失败，先停下并按第 10 节排查；不要连续随机重试。
7. 完成后按第 11 节把结果回传给人类。

## 2. 选择分支

只有两条分支值得关注，都不需要另开 PR 才能跑：

| 目的 | 分支 | 说明 |
|---|---|---|
| 稳妥：跑已经过评审的基线 | `main` | PR #1–#5 全在里面：工作台、材料全文阅读、LLM 网关、检索链路 |
| 完整：连带一键脚本与最新检索 | `feat/rebuild-rag-pipeline` | 在 `main` 之上多一个提交；`启动项目.cmd`、`scripts/launch.mjs`、`scripts/reindex-all.mjs` 只在这条分支 |

老分支 `feat/llm-gateway` / `feat/workspace-material-reader` 已随 PR #3、#4 合并，内容都在 `main` 里，**不要再切**。

## 3. 前置条件

需要：

- Git
- Node.js ≥ 20（只用来执行仓库脚本）
- Docker Desktop 或 Docker Engine + Compose v2，并且已经启动
- 宿主机 `8080` 与 `13306` 端口空闲
- 能访问 GitHub 与 Docker Hub；首次构建需要拉取基础镜像

Windows 建议使用 **Git Bash**。PowerShell 中的 `curl` 是别名，参数行为与本文不同，不要直接照抄。

先检查：

```bash
node --version
docker --version
docker compose version
docker info >/dev/null && echo "Docker OK"
```

期望：Node 主版本 ≥ 20；Docker 命令可用；最后输出 `Docker OK`。

如果只部署运行，**不需要安装 Go**，也**不需要执行 `npm install`**。Go 只在跑质量门禁或本地直接运行后端时需要。

## 4. 获取代码

```bash
git clone https://github.com/Corday-nkusky/team_project.git
cd team_project

# 二选一：按第 2 节选择（默认已在 main）
git switch feat/rebuild-rag-pipeline
# 或
git switch main

git log -1 --oneline
```

期望：切换成功，`git log -1` 显示当前分支最新提交。

## 5. 一条命令启动

在仓库根目录执行：

```bash
node scripts/up.mjs
```

脚本会：

1. 从 `deploy/.env.example` 生成 `deploy/.env`；
2. 为 `JWT_SECRET`、`MYSQL_PASSWORD`、`MYSQL_ROOT_PASSWORD` 填入随机值；
3. 执行 `docker compose up -d --build`，起四个服务：`mysql` / `qdrant` / `backend`（Web）/ `worker`（索引消费者，独立进程）；
4. 轮询 `http://127.0.0.1:8080/health`，就绪后打印版本与依赖状态。

Windows 上也可以直接双击根目录的 `启动项目.cmd`（内部就是这条命令，额外会检查 Docker 引擎、等它就绪后自动开浏览器）；对应地有 `停止项目.cmd`，它执行 `docker compose down`，**数据卷保留**。

首次构建会拉取 `node:22-alpine`、`golang:1.27`、`mysql:8.4`、`qdrant/qdrant:v1.12.4` 等镜像，可能需要数分钟到十几分钟。

期望的最后几行类似：

```text
已生成 deploy/.env
已写入随机值：JWT_SECRET, MYSQL_PASSWORD, MYSQL_ROOT_PASSWORD
...
就绪：version=dev time=2026-11-01T...Z deps={"mysql":"ok","qdrant":"ok"}
```

看到 `就绪` 后，验证：

```bash
curl -sS --noproxy '*' http://127.0.0.1:8080/health
```

期望：HTTP 200，`deps.mysql` 与 `deps.qdrant` 都是 `ok`。此时没有配置模型 key，通常是 `"llm":{"enabled":false}`，这是正常的抽取式降级模式。

如果 `curl` 走了代理导致失败，保留 `--noproxy '*'` 再试。

## 6. 浏览器与演示账号

打开：

```text
http://127.0.0.1:8080
```

演示账号（启动时自动写入，幂等）：

| 账号 | 口令 | 角色 | 班级 |
|---|---|---|---|
| `teacher1` | `teacher123` | 教师 | 软件工程1班 |
| `student1` | `student123` | 学生 | 软件工程1班 |
| `teacher2` | `teacher123` | 教师 | 软件工程2班 |
| `student2` | `student123` | 学生 | 软件工程2班 |

最小冒烟验证：

1. 用 `teacher1` 登录，上传一条包含独有词（例如 `CAMPUSCLAW_SMOKE_2026`）的文本材料。
   上传立刻返回 202，材料先显示「索引中」，由 `worker` 异步切片后转为 `ready`（通常几秒）。
   如果一直停在「索引中」：`cd deploy && docker compose ps worker` 确认消费者在跑，再用
   `docker compose logs --tail=50 worker` 看它报什么。
2. 用 `student1` 登录，提问 `CAMPUSCLAW_SMOKE_2026 是什么？`，应看到流式回答和材料引用。
3. 再问一个与材料无关的问题（例如 `如何在家做红烧肉？`），应返回固定文案 `资料中未找到相关内容`，而不是编造答案。

`teacher2` / `student2` 属于另一个班，可用于验证班级隔离：2 班看不到 1 班的材料。

## 7. 接入课程网站提供的模型 API

### 7.1 先获取什么

登录课程网站，在课程资源、课程公告或专门的「大模型 API / 模型网关」页面获取四项。课程网站入口以课程群或课程公告中的链接为准；仓库不记录该地址。

| 配置项 | 填什么 | 说明 |
|---|---|---|
| `LLM_API_BASE` | 课程网站给出的网关根地址 | 必须包含 `/v1` 等前缀，但不要带 `/chat/completions`；代码会自动拼接端点 |
| `LLM_API_KEY` | 课程网站给出的 API Key | 个人密钥，只放本机 `deploy/.env` |
| `LLM_CHAT_MODEL` | 课程网站给出的对话模型名 | 本次课程环境实测为 `course-chat`，以网站说明为准 |
| `LLM_EMBED_MODEL` | 课程网站给出的嵌入模型名 | 本次课程环境实测为 `course-embedding`，以网站说明为准 |

课程网关必须**同时支持** `/chat/completions`（SSE 流式）和 `/embeddings`。只支持对话、不支持嵌入的网关不能直接用于本项目的材料索引。

### 7.2 填入环境

用编辑器打开：

```text
deploy/.env
```

只修改下面四行，保留文件中已有的随机 MySQL / JWT 配置：

```dotenv
LLM_API_BASE=<课程网站给出的网关根地址，以 /v1 结尾>
LLM_API_KEY=<课程网站给出的 API Key>
LLM_CHAT_MODEL=course-chat
LLM_EMBED_MODEL=course-embedding
```

注意事项：

- 四个值必须同时填写。只填 key 不填 base 或模型名，服务会在监听端口前退出，日志只报缺失的键名。
- 模型名不要猜。课程网站写的是什么，就填什么；本次课程环境是 `course-chat` / `course-embedding`。
  仓库 `deploy/.env.example` 里的智谱示例（`glm-5.3-flash` / `embedding-3`）只是示例，用课程给的值覆盖即可；
  智谱 v4 的 Key 必须是 `id.secret` 形式，客户端会自动签发 JWT（见 README「智谱 v4 的认证」）。
- `LLM_API_KEY` 不要写进 README、issue、聊天记录或任何提交。
- 如果暂时没有 key，保持 `LLM_API_KEY` 为空即可，系统仍可完成上传、检索、抽取式问答和引用；只是不启用模型生成与向量检索。

### 7.3 重启并验证

修改 `.env` 后必须让容器重新读取环境变量：

```bash
cd deploy
docker compose up -d
```

不要用 `docker compose restart backend` 代替；`restart` 不会重新注入新的环境变量。

等待后端就绪，再检查：

```bash
curl -sS --noproxy '*' http://127.0.0.1:8080/health
```

期望看到：

- `llm.enabled` 为 `true`；
- 还会返回 `llm.chat_model` 与 `llm.embed_model`，应与 `.env` 中填写的一致；
- 响应中不应出现 key、base 或连接串。

如果仍是 `enabled=false`，检查 `.env` 是否改在 `deploy/.env`、key 是否为空，以及是否执行了 `docker compose up -d`。

### 7.4 全链路验证

仓库自带验证脚本，在仓库根目录执行：

```bash
node scripts/verify-llm.mjs
```

它会用演示账号完成「健康检查 → 教师上传 → 索引 ready → 学生提问 → SSE 引用」的完整链路，并打印逐项结果。预期全部 `ok`，退出码 0。

如果脚本不存在（很旧的 clone），就手工验证：

1. 用 `teacher1` 上传一条材料，确认状态为 `ready`；
2. 用 `student1` 提问材料中的独有词，确认回答是流式的且带引用；
3. 查看 `/health` 的 `llm.enabled` 是否为 `true`。

### 7.5 重要：先配 key，再灌语料

材料是在上传/建索引时生成向量的：

- 先配好 key 再上传的材料：会写入向量，可以使用向量检索；
- 先在没有 key 时上传的材料：只有关键词索引，没有向量；
- 后补 key 后，**新上传**的材料才会自动带向量；旧材料需要重索引才会进入向量路。

因此推荐顺序是：**先完成第 7 节的模型配置，再执行第 8 节的语料灌入。**

如果已经先灌了语料再配 key，有两种处理：

1. 演示环境最省事：清空数据后重来（会删除数据库与向量，谨慎）：
   ```bash
   cd deploy
   docker compose down -v
   cd ..
   node scripts/up.mjs
   # 再重新执行第 8 节
   ```
2. 保留数据：以教师身份调用 `POST /api/materials/{id}/reindex` 逐条重建索引；当前界面只对 `failed` 材料显示「重试索引」按钮，`ready` 的旧材料不会显示。

如果以后更换了嵌入模型或向量维度，需要先删除 Qdrant 集合 `team_campusclaw_chunks`，再重索引全部材料；新旧维度不能混用。

## 8. 可选：灌入演示语料

应用启动后，在另一个终端执行：

```bash
cd seed-data/biology
node seed.mjs
```

脚本会以 `teacher1` 身份把约 100 条生物学材料灌入 1 班，按标题幂等跳过已存在的材料，可重复执行。

注意事项：

- 需要应用已经在 `http://127.0.0.1:8080` 运行；
- 启用模型后，灌语料会调用约 100 次嵌入请求，注意课程 API 的额度与限流；
- 如果中途失败，重新执行即可，已成功的标题会跳过；
- 运行后生成的 `seed-data/biology/manifest.json` 是本机记录，已被 git 忽略，不需要提交；
- 灌完后在材料列表确认状态为 `ready`，失败的可以点「重试索引」。

如果只想快速验证，先用 1–2 条材料测试，不要一上来灌全量。

## 9. 可选：质量门禁与端到端

只部署运行不需要本节。需要提交代码或做评审证据时再执行。

质量门禁（需要 Go 1.27+）：

```bash
node scripts/quality-gate.mjs
```

Go 不在 PATH 时，按 README 使用 `GO_BIN` 指定，例如：

```bash
GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs
```

加 OpenSpec 严格校验：

```bash
node scripts/quality-gate.mjs --with-spec
```

Go 接口 e2e（服务已启动，且需要本机 MySQL 连接信息）：

```bash
cd backend
set -a; source ../deploy/.env; set +a
MYSQL_HOST=127.0.0.1 MYSQL_PORT=13306 go run ./cmd/e2e
```

浏览器 e2e（需要本机 Chrome）：

```bash
node scripts/e2e-ui.mjs
```

Windows 默认读取 `C:/Program Files/Google/Chrome/Application/chrome.exe`；路径不同时用 `CHROME` 指定。

## 10. 常见问题

| 现象 | 原因 | 处理 |
|---|---|---|
| compose 报 `JWT_SECRET 未设置` | `.env` 缺失或仍是占位值 | 回到仓库根目录执行 `node scripts/up.mjs`，不要手工复制占位值 |
| 后端启动即退出，日志报某个键名 | 必填配置缺失或非法 | 这是预期行为；按日志补 `.env`，不要改成默认值绕过 |
| `8080` 或 `13306` 被占用 | 已有服务占用端口 | 停掉占用端口的服务；本项目固定使用 8080 / 13306 |
| `docker info` 失败 | Docker Desktop 没启动 | 启动 Docker Desktop，等状态变为 running |
| 首次构建很慢或拉镜像失败 | 网络或 Docker Hub 限速 | 配置 Docker 镜像加速后重试；不要中断正在进行的构建 |
| `/health` 返回 503 | MySQL 或 Qdrant 未就绪 | `cd deploy && docker compose logs mysql qdrant backend`，等待或修复依赖 |
| 登录失败 | 种子数据未写入 | 查看后端日志；重启后端会重新执行幂等种子 |
| `llm.enabled=false` | key 为空、改错文件或未重建容器 | 检查 `deploy/.env`，执行 `cd deploy && docker compose up -d`，不要只 `restart` |
| 模型接口 401 / 403 / 429 | key 错误、过期、额度不足或限流 | 回课程网站核对 key 与额度；不要把自己的 key 发给别人 |
| 模型接口 404 或模型不存在 | 模型名与课程网关不匹配 | 按课程网站重新填写 `LLM_CHAT_MODEL` / `LLM_EMBED_MODEL` |
| 材料一直 `failed`，错误含嵌入失败 | key、模型名或嵌入端点有问题 | 修复 `.env` 后重索引；如果换过模型维度，先删 Qdrant 集合 |
| 材料是旧数据，提问没有向量召回 | 材料在启用 key 前上传 | 重索引旧材料，或在清空数据后按「先配 key，再灌语料」重来 |
| 不需要重新 `npm install` | 前端在 Docker 镜像内构建 | 只有改前端、跑 `vite dev` 时才需要进入 `frontend/` 安装依赖 |

### 数据与停止

```bash
cd deploy
docker compose down          # 停止容器，保留 MySQL / Qdrant 数据
docker compose down -v       # 连数据卷一起删除（不可恢复）
```

数据库和向量分别保存在 Docker 卷 `deploy_mysql-data`、`deploy_qdrant-data`，不在 Git 仓库里。一旦初始化过数据库，不要随意删除或重建 `deploy/.env`：MySQL 口令在首次初始化时已写入数据卷，改了 `.env` 会让后端连不上。要彻底重置就使用 `down -v` 后重新执行 `node scripts/up.mjs`。

## 11. 完成后回传什么

请把下面的信息发回给人类，不要附带 `.env` 内容或任何 key：

```text
环境：操作系统 / Docker 版本 / Node 版本
分支与提交：git log -1 --oneline 的输出
启动：node scripts/up.mjs 的退出码与最后几行
健康检查：/health 的 HTTP 状态、deps、llm.enabled
界面：登录 / 上传 / 提问 / 引用是否通过
模型（如启用）：chat_model、embed_model，verify-llm.mjs 是否全绿
失败项：卡在哪一步、完整错误（先遮住 key）、截图或日志
```

如果只是部署运行，到第 8 节结束即可。要作为课程评审证据，再执行第 9 节并把命令、日期、退出码与实际输出写进 `docs/lesson-07/peer-review.md`，由执行人本人签字，不要代签。
