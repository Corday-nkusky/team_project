# Spec Delta

## Purpose

上传与重索引不再在请求内建索引：提交接口同事务写入任务并返回 202，索引由独立消费者完成，结果凭任务标识查询。输入边界、班级隔离、审计与界面状态要求不变；「可检索」从响应里的即时状态改为任务终态后的可观测结果。

## MODIFIED Requirements

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
