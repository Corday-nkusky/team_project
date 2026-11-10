# Spec Delta

## Purpose

让服务在启动时就能暴露配置错误，让编排能用一个免认证端点判断依赖是否就绪，让每条请求在日志里可被唯一串联且不泄漏敏感值；
本次补充服务端错误的 ERROR 日志，以及 404 / 405 / 413 / 415 / 尾随 JSON 的请求契约。

## MODIFIED Requirements

### Requirement: R10 结构化日志与请求标识

服务 SHALL 为每条 HTTP 请求输出一条单行 JSON 访问日志，字段至少含 `request_id`、`method`、`path`、`status`、`duration_ms`。响应头 SHALL 回写 `X-Request-ID`；客户端提供的 ID 仅在长度为 1–128 且全部为可打印 ASCII 时透传，否则重新生成。日志 SHALL NOT 包含口令、令牌、`Authorization` 头或请求体中的敏感值。
返回 5xx 的请求 SHALL 额外产生一条 `level=ERROR` 的日志，含 `request_id`、`operation` 与底层错误，便于定位。

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

#### Scenario: R10.6 服务端错误留 ERROR 日志

- **GIVEN** 某接口因数据库、检索或模型保存等内部错误返回 500
- **WHEN** 查看运行日志
- **THEN** 至少有一条 `level=ERROR` 的日志，含非空 `request_id`、`operation` 与 `error`
- **AND** 该日志不含请求体、`Authorization`、令牌、口令或完整 SQL 语句

## ADDED Requirements

### Requirement: R11 请求与错误契约

所有 `/api/` 路径 SHALL 使用统一 JSON 错误体。未注册的 API 路径 SHALL 返回 404；已注册路径的未支持方法 SHALL 返回 405 并给出正确的 `Allow`；请求体超限 SHALL 返回 413；内容类型不符 SHALL 返回 415；一个请求体中出现尾随 JSON 内容 SHALL 返回 400。以上错误 SHALL NOT 回落到 SPA 页面或纯文本。

#### Scenario: R11.1 未支持的方法返回 405 JSON

- **GIVEN** `/api/ask` 只接受 `POST`
- **WHEN** 以 `GET` 或 `PUT` 请求 `/api/ask`
- **THEN** 返回 405，`Content-Type` 为 `application/json`
- **AND** 响应体为 `{"error":{"code":"method_not_allowed",...}}`
- **AND** `Allow` 头只包含 `POST`

#### Scenario: R11.2 未知 API 路径返回 404 JSON

- **GIVEN** 服务已启动
- **WHEN** 请求不存在的 `/api/not-a-route`
- **THEN** 返回 404，`Content-Type` 为 `application/json`
- **AND** 响应体为统一错误结构，不返回 SPA 的 `index.html`

#### Scenario: R11.3 请求体超限返回 413

- **GIVEN** 问答接口的 JSON 请求体上限为 1 MiB
- **WHEN** 提交 1.1 MiB 的请求体
- **THEN** 返回 413，错误码为 `payload_too_large`
- **AND** 不调用检索或模型

#### Scenario: R11.4 内容类型不符返回 415

- **GIVEN** 接口期望 `application/json`
- **WHEN** 以 `Content-Type: text/plain` 提交相同内容
- **THEN** 返回 415，错误码为 `unsupported_media_type`
- **AND** 不解析请求体

#### Scenario: R11.5 尾随 JSON 内容被拒

- **GIVEN** 问答接口只接受一个 JSON 对象
- **WHEN** 提交 `{"question":"..."}{"x":1}`
- **THEN** 返回 400，错误码为 `bad_request`
- **AND** 不开始 SSE、不调用检索或模型
