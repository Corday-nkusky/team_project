# Spec Delta

## Purpose

让服务在启动时就能暴露配置错误，让编排能用一个免认证端点判断依赖是否就绪，让每条请求在日志里可被唯一串联且不泄漏敏感值。检索问答与班级隔离属于既有能力，见 `specs/auth-upload/spec.md` 与 `specs/knowledge-retrieval/spec.md`。

## ADDED Requirements

### Requirement: R8 配置外置与启动校验

系统 SHALL 从环境变量读取部署关键配置（`HTTP_ADDR`、`MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_DATABASE`、`QDRANT_URL`、`JWT_SECRET`）。代码与镜像 SHALL NOT 内置这些项的字面量或可用默认值。任一必需项缺失、为空或格式非法时，进程 SHALL 在监听端口前以非零码退出，并输出一条 JSON 日志指明缺失/非法的键名；日志 SHALL NOT 输出该键的值。`LLM_API_KEY` 为空是合法配置，表示问答走抽取式降级。

#### Scenario: R8.1 配置齐全时启动

- **GIVEN** 环境变量提供全部必需项，且 MySQL 与 Qdrant 可用
- **WHEN** 启动服务
- **THEN** 进程进入监听状态，`GET /health` 返回 200
- **AND** 启动日志为单行 JSON

#### Scenario: R8.2 缺少 JWT_SECRET 时拒绝启动

- **GIVEN** 除 `JWT_SECRET` 外其余必需项齐全
- **WHEN** 启动服务
- **THEN** 进程以非零码退出
- **AND** 日志含键名 `JWT_SECRET`，不含该键的值

#### Scenario: R8.3 缺少 MySQL 连接项时拒绝启动

- **GIVEN** `MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_USER`、`MYSQL_PASSWORD` 中任一项为空
- **WHEN** 启动服务
- **THEN** 进程以非零码退出
- **AND** 日志逐项指明缺失的键名

#### Scenario: R8.4 示例 JWT 密钥被拒绝

- **GIVEN** `JWT_SECRET` 等于示例值 `dev-insecure-secret-change-me` 或 `replace-with-openssl-rand-hex-32`
- **WHEN** 启动服务
- **THEN** 进程以非零码退出
- **AND** 日志说明该值不能用于运行

#### Scenario: R8.5 非法端口被拒绝

- **GIVEN** `MYSQL_PORT` 不是整数，或不在 1–65535 范围内
- **WHEN** 启动服务
- **THEN** 进程以非零码退出
- **AND** 日志指明 `MYSQL_PORT` 非法

#### Scenario: R8.6 无模型密钥时降级启动

- **GIVEN** 必需项齐全，且 `LLM_API_KEY` 为空
- **WHEN** 启动服务
- **THEN** 进程正常监听
- **AND** `GET /health` 的 `llm.enabled` 为 `false`

### Requirement: R9 健康检查

系统 SHALL 提供免认证的 `GET /health`。依赖全部连通时 SHALL 返回 200；任一外部依赖不可用时 SHALL 返回 503。响应体 SHALL 同时包含 `status`、`version`、`time`（UTC RFC3339）、`deps.mysql`、`deps.qdrant`；SHALL NOT 包含 `JWT_SECRET`、数据库口令或模型密钥。

#### Scenario: R9.1 依赖就绪时返回完整探活信息

- **GIVEN** MySQL 与 Qdrant 均可用
- **WHEN** 不带 `Authorization` 头请求 `GET /health`
- **THEN** 返回 200
- **AND** 响应含非空 `version`、可解析的 `time`，且 `deps.mysql` 与 `deps.qdrant` 均为 `ok`

#### Scenario: R9.2 MySQL 不可用时返回 503

- **GIVEN** MySQL 不可达，Qdrant 可用
- **WHEN** 请求 `GET /health`
- **THEN** 返回 503
- **AND** `deps.mysql` 为非 `ok` 的原因，`deps.qdrant` 仍为 `ok`

#### Scenario: R9.3 Qdrant 不可用时返回 503

- **GIVEN** Qdrant 不可达，MySQL 可用
- **WHEN** 请求 `GET /health`
- **THEN** 返回 503
- **AND** `deps.qdrant` 为非 `ok` 的原因，`deps.mysql` 仍为 `ok`

#### Scenario: R9.4 未登录也可探活

- **GIVEN** 请求未携带任何令牌
- **WHEN** 请求 `GET /health`
- **THEN** 返回 200 或 503
- **AND** SHALL NOT 返回 401 或 403

#### Scenario: R9.5 探活响应不泄漏密钥

- **GIVEN** 服务已配置 `JWT_SECRET`、`MYSQL_PASSWORD` 与 `LLM_API_KEY`
- **WHEN** 请求 `GET /health` 并检查完整响应体
- **THEN** 响应体不含上述任一配置值
- **AND** 响应体不含 `password`、`secret`、`api_key` 字段

### Requirement: R10 结构化日志与请求标识

服务 SHALL 为每条 HTTP 请求输出一条单行 JSON 访问日志，字段至少含 `request_id`、`method`、`path`、`status`、`duration_ms`。响应头 SHALL 回写 `X-Request-ID`；客户端提供的 ID 仅在长度为 1–128 且全部为可打印 ASCII 时透传，否则重新生成。日志 SHALL NOT 包含口令、令牌、`Authorization` 头或请求体中的敏感值。

#### Scenario: R10.1 每条请求产生一条 JSON 日志

- **GIVEN** 服务已启动
- **WHEN** 发送一次 `GET /api/materials`
- **THEN** 标准输出新增一条可被 `JSON.parse` 解析的日志
- **AND** 该行含非空 `request_id`、`method=GET`、`path=/api/materials`、数值 `status` 与 `duration_ms`

#### Scenario: R10.2 客户端 request ID 被透传

- **GIVEN** 客户端在请求头提供 `X-Request-ID: peer-123`
- **WHEN** 请求任意接口
- **THEN** 响应头 `X-Request-ID` 为 `peer-123`
- **AND** 对应访问日志的 `request_id` 为 `peer-123`

#### Scenario: R10.3 登录失败日志不含口令

- **GIVEN** 客户端提交错误口令
- **WHEN** 登录接口返回 401
- **THEN** 访问日志与审计日志均不含该口令明文
- **AND** 日志只记录动作、结果与必要的非敏感标识

#### Scenario: R10.4 SSE 请求只记一条访问日志

- **GIVEN** 客户端发起一次流式问答并正常结束
- **WHEN** 检查访问日志
- **THEN** 该请求只产生一条 `path=/api/ask` 的访问日志
- **AND** 日志在流结束后写出，`status` 为 200

#### Scenario: R10.5 非法 request ID 被替换

- **GIVEN** 客户端提供的 `X-Request-ID` 超过 128 字符，或含不可打印字符
- **WHEN** 请求任意接口
- **THEN** 服务重新生成一个合法 ID 并在响应头返回
- **AND** 日志中的 `request_id` 与响应头一致
