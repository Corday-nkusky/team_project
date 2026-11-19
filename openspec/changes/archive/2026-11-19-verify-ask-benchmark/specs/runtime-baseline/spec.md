## ADDED Requirements

### Requirement: 问答压测验证流式完成

问答压测 SHALL 按事件判断回答、无依据拒答及失败，并分别报告首字与完成耗时。

#### Scenario: 正常回答或拒答

- **GIVEN** 返回合法 meta，命中时有引用，拒答时引用为空
- **WHEN** 收到可见正文（拒答为约定文案）与 done
- **THEN** 对应类别加一，首字与完成时间纳入统计

#### Scenario: HTTP 200 后回答失败

- **GIVEN** 问答接口已返回 HTTP 200
- **WHEN** 流显式报错、缺少 done、正文或要求的引用
- **THEN** 计为失败，脚本以非零状态退出，失败样本不计入完成时延指标
