## ADDED Requirements

### Requirement: F6 镜像版本烟雾检查

CI 的版本镜像烟雾检查 SHALL 在无 MySQL/Qdrant 的隔离 job 中显式使用 `REQUIRE_DEPS=false` 启动容器，并断言 `/health` 返回的 `version` 等于注入的提交标识。该检查 SHALL 接受依赖不可用导致的 503 降级响应，且 SHALL NOT 将其记录为依赖全就绪。验证失败时，CI SHALL 在删除容器前输出容器日志。

#### Scenario: F6.1 无依赖环境仍可核对版本

- **GIVEN** 镜像已注入提交标识，job 未启动 MySQL/Qdrant
- **WHEN** 以 `REQUIRE_DEPS=false` 启动容器并轮询 `/health`
- **THEN** 服务开始监听并返回合法 JSON，HTTP 状态为 503
- **AND** 响应的 `version` 与注入的提交标识一致
- **AND** `deps` 如实标明依赖不可用，job 的版本断言退出码为 0

#### Scenario: F6.2 启动或版本断言失败可诊断

- **GIVEN** 容器未成功监听，或 `/health.version` 与注入值不一致
- **WHEN** 版本验证步骤失败
- **THEN** job 以非零状态结束
- **AND** 删除容器前输出 `docker logs campusclaw-version`
- **AND** 不仅保留空响应导致的 JSON 解析错误
