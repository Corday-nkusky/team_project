# auth-upload Specification

## Purpose
让每个账号只能在自己的班级内登录、上传与访问教学材料：越权访问被明确拒绝，关键操作留有可追溯记录。
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

教师 SHALL 能将教学材料（纯文本 / Markdown / PDF 文本）上传到本班；系统 SHALL 落库原文并
建立可检索索引。上传 SHALL 记入操作留痕。

#### Scenario: R3.1 上传成功并可检索

- **GIVEN** 教师已登录且归属班级 A；检索引擎（Qdrant）可用
- **WHEN** 教师上传一份非空材料
- **THEN** 返回 201，材料落库且其切片索引状态为 `ready`
- **AND** 随即在本班检索其独有词，能命中该材料

#### Scenario: R3.2 空材料被拒

- **GIVEN** 教师已登录；记下班级 A 当前材料条数
- **WHEN** 上传内容为空（或仅空白字符）
- **THEN** 返回 400，且不落库

#### Scenario: R3.3 索引失败可被察觉

- **GIVEN** 检索引擎在建立索引时不可用（后端可连库，但 Qdrant 不可达）
- **WHEN** 检索引擎在建立索引时不可用
- **THEN** 材料被标记为 `failed`（而非静默成功），且可通过重试接口重新索引

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
| `auth-upload` | R1.1–R1.7、R2.1–R2.3、R3.1–R3.3、R4.1–R4.2 |
| `knowledge-retrieval` | R5.1–R5.3、R6.1–R6.3、R7.1–R7.3 |
| `runtime-baseline` | R8.1–R8.6、R9.1–R9.5、R10.1–R10.5 |
