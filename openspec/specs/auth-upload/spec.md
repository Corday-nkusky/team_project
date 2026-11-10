# auth-upload Specification

## Purpose
让每个账号只能在自己的班级内登录、上传与访问教学材料：越权访问被明确拒绝，关键操作留有可追溯记录。
上传前须校验标题长度、正文编码、文件类型与体积，并与数据库列容量对齐；界面上的文件上传与材料列表须给出可辨认的加载、空与错误状态。
检索问答与引用回看属于另一项能力，见 `specs/knowledge-retrieval/spec.md`。

## Requirements

### Requirement: R1 账号口令认证与登录态

系统 SHALL 提供账号口令登录与登出，账号只有**教师**与**学生**两种角色。登录成功 SHALL 同时返回
**短期 access token** 与**长期 refresh token**；后续受保护请求通过 `Authorization: Bearer <access token>` 头携带。
access token 过期后，客户端 SHALL 凭 refresh token 换取新的 access token。登出 SHALL 使 refresh token 失效。

#### Scenario: R1.1 正确凭据登录成功

- **GIVEN** 班级 A 中存在一个已激活的教师账号，口令已知
- **WHEN** 用户提交存在且口令正确的账号
- **THEN** 返回 200，响应体含 `access_token` 与 `refresh_token`
- **AND** 响应体不含口令哈希或任何口令相关字段

#### Scenario: R1.2 失败响应同形

- **GIVEN** 班级 A 中存在账号 `teacher_a`；另有用户名 `ghost_user` 未被注册
- **WHEN** 分别提交「账号不存在」「口令错误」两类请求
- **THEN** 两者的状态码、响应体结构**不可区分**（均为 401，文案一致）

#### Scenario: R1.3 篡改的 access token 被拒绝

- **GIVEN** 用户已用正确凭据登录，持有一枚未过期的 access token
- **WHEN** 对已签发的 access token 做任意一字节改动后携带它请求受保护接口
- **THEN** 返回 401

#### Scenario: R1.4 未携带凭证访问受保护接口

- **GIVEN** 服务已启动，受保护接口 `/api/me`、`/api/materials`、`/api/ask` 可用
- **WHEN** 不带 `Authorization` 头请求任一受保护接口
- **THEN** 返回 401

#### Scenario: R1.5 登出使 refresh token 失效

- **GIVEN** 用户已登录，同时持有 access token 与 refresh token
- **WHEN** 已登录用户调用登出
- **THEN** 返回 200，且该 refresh token 随即无法再换取 access token
- **AND** 已签发的 access token 在自身有效期内仍可用（无状态凭证的已知取舍，见 design）

#### Scenario: R1.6 凭 refresh token 换取新的 access token

- **GIVEN** 用户已登录，其 refresh token 未被使用、未过期、未登出
- **WHEN** 客户端以有效的 refresh token 请求刷新
- **THEN** 返回 200 与新的 access token
- **AND** 同时返回**新的 refresh token**，且旧的 refresh token 立即失效（轮换）

#### Scenario: R1.7 失效的 refresh token 被拒绝

- **GIVEN** 一枚 refresh token 已处于失效状态（已过期 / 已登出 / 已被轮换，三者之一）
- **WHEN** 使用已失效的 refresh token（已过期 / 已登出 / 已被轮换）请求刷新
- **THEN** 返回 401，客户端须重新登录

### Requirement: R2 角色与班级隔离

账号分为**教师**与**学生**两种角色，且归属某个班级。系统 SHALL 拒绝任何跨班请求：
教师不能写入他班材料，任何人不能读取他班材料。

#### Scenario: R2.1 跨班读被拒

- **GIVEN** 班级 A 的学生已登录；班级 B 存在一份材料及其 id
- **WHEN** 班级 A 的用户请求班级 B 的材料（直接构造 B 的 id）
- **THEN** 返回 403 或 404，且响应体**不含** B 的任何内容

#### Scenario: R2.2 跨班检索不泄漏

- **GIVEN** 班级 B 的材料含独有词 `X`，班级 A 的全部材料均不含 `X`
- **WHEN** 班级 A 的学生发起检索，而匹配内容只存在于班级 B 的材料
- **THEN** 结果为空（不返回 B 的任何片段或标题）

#### Scenario: R2.3 学生不能写材料

- **GIVEN** 学生角色的账号已登录
- **WHEN** 角色为学生的账号调用上传接口
- **THEN** 返回 403

### Requirement: R3 教学材料上传与索引

教师 SHALL 能将教学材料（纯文本 / Markdown / PDF 文本）上传到本班；系统 SHALL 落库原文、写入索引任务并记入操作留痕。提交 SHALL 返回 `202` 与 `task_id`、`status_url`，响应体不含索引结果；索引由独立消费者在请求之外执行，任务 `succeeded` 后材料才可被本班检索。系统 SHALL 在落库前校验标题长度、正文编码、文件类型与体积；任一校验失败 SHALL 返回可判定的 4xx、不落库、不产生任务。

#### Scenario: R3.1 上传成功并可检索

- **GIVEN** 教师已登录且归属班级 A；检索引擎（Qdrant）可用，消费者已启动
- **WHEN** 教师上传一份非空材料
- **THEN** 返回 202，响应体含 `task_id` 与 `status_url`，且不含 `ready`、切片数或任何索引结果
- **AND** 轮询状态接口至 `succeeded` 后，材料 `index_status` 为 `ready`，随即在本班检索其独有词能命中该材料

#### Scenario: R3.2 空材料被拒

- **GIVEN** 教师已登录；记下班级 A 当前材料条数
- **WHEN** 上传内容为空（或仅空白字符）
- **THEN** 返回 400，且不落库

#### Scenario: R3.3 索引失败可被察觉

- **GIVEN** 检索引擎在建立索引时不可用（后端可连库，但 Qdrant 不可达）
- **WHEN** 该材料的索引任务耗尽重试次数转为 `dead`，或判定为不可重试失败
- **THEN** 任务行保留 `error` 与 `attempts`，材料被标记为 `failed`（而非静默成功）
- **AND** 通过重索引接口重放同一任务后，任务回到 `pending`，无需新建材料

#### Scenario: R3.4 上传输入边界

- **GIVEN** 教师已登录；记下班级 A 当前材料条数
- **WHEN** 提交标题超过 255 个字符、正文不是合法 UTF-8、正文超过 32 MiB，或文件扩展名不在 `.txt / .md / .markdown / .pdf` 内
- **THEN** 分别返回 400 `title_too_long`、400 `invalid_encoding`、413 `payload_too_large`、415 `unsupported_file_type`
- **AND** 错误体为统一结构 `{"error":{"code","message"}}`
- **AND** 四种情况均不落库，材料条数与任务条数不变

#### Scenario: R3.5 上传容量与数据库列对齐

- **GIVEN** 服务已完成启动迁移
- **WHEN** 检查 `materials.content` 的列类型，并上传 17 MiB 合法文本
- **THEN** 列类型为 `LONGTEXT`（容量远大于 32 MiB）
- **AND** 17 MiB 文本返回 202；轮询任务至 `succeeded` 后索引状态为 `ready`，不出现「落库失败」
- **AND** 超过 32 MiB 的请求返回 413，而不是 500

#### Scenario: R3.6 从界面选择文件可上传

- **GIVEN** 教师已登录并打开「教学材料」页
- **WHEN** 通过文件选择器选择一个 `.md` 文件
- **THEN** 请求以 `multipart/form-data` 发送，`Content-Type` 含浏览器生成的 boundary
- **AND** 返回 202；界面轮询任务至终态后刷新列表，该材料出现且状态为「可检索」

#### Scenario: R3.7 材料列表的加载、空与错误态

- **GIVEN** 任意角色已登录并进入「教学材料」页
- **WHEN** 列表请求进行中、返回空列表或请求失败
- **THEN** 加载中显示「加载中」占位，不显示误导性的「暂无材料」
- **AND** 空态给出下一步：教师看到上传引导，学生看到「请联系教师上传」
- **AND** 错误态显示错误信息与「重试」按钮，点击重试后重新加载

#### Scenario: R3.8 长标题与窄视口下表格不塌陷

- **GIVEN** 本班存在标题为 255 个字符的材料
- **WHEN** 在 524px 宽的视口查看材料列表
- **THEN** 「索引状态」表头与状态徽标保持单行，不竖排成单字
- **AND** 标题最多显示两行，行高不再因标题长度膨胀到普通行的数倍
- **AND** 完整标题可通过悬停提示或展开查看，不丢失信息

### Requirement: R4 操作留痕（审计最小实现）

系统 SHALL 对登录、材料上传、检索问答三类动作各写一条可追溯记录，含时间、操作者、动作与结果。

#### Scenario: R4.1 三类动作各留痕

- **GIVEN** 审计表可用，且已记下当前记录总数
- **WHEN** 依次发生一次成功登录、一次材料上传、一次问答
- **THEN** 审计表新增三条对应记录，均含操作者标识与时间戳

#### Scenario: R4.2 失败动作亦留痕

- **GIVEN** 存在账号 `teacher_a`，其正确口令已知
- **WHEN** 发生一次登录失败
- **THEN** 审计表记录该失败及其原因类别（不记录明文口令）

## 能力与场景对照

| 能力 | 场景 |
| --- | --- |
| `auth-upload` | R1.1–R1.7、R2.1–R2.3、R3.1–R3.8、R4.1–R4.2 |
| `async-indexing` | A1.1–A1.3、A2.1–A2.3、A3.1–A3.3、A4.1–A4.3、A5.1–A5.4、A6.1–A6.2、A7.1–A7.2、A8.1–A8.3 |
| `knowledge-retrieval` | R5.1–R5.9、R6.1–R6.6、R7.1–R7.7 |
| `runtime-baseline` | R8.1–R8.6、R9.1–R9.5、R10.1–R10.6、R11.1–R11.5、R12.1–R12.3 |
| `learning-workspace` | LW1.1–LW1.2、LW2.1–LW2.2、LW3.1–LW3.2、LW4.1–LW4.2 |
| `material-reading` | MR1.1–MR1.4、MR2.1–MR2.6 |
| `ask-history` | H1.1–H1.4、H2.1–H2.3、H3.1–H3.3 |
