# skills Specification

## Purpose

教师按班维护的「解题引导」技能在提问时由服务端加载，改变回答的引导方式；技能的开关与版本可追踪，按班隔离，学生无法配置。

## Requirements

### Requirement: SK1 教师按班维护解题引导技能

系统 SHALL 提供需要教师登录的 `GET /api/skills`（本班列表）、`POST /api/skills`（新建或按 id 更新）、`DELETE /api/skills/{id}`（删除）接口。技能字段 SHALL 含 name、kind（`solve_hint`）、enabled、instruction（提示词）、tool_refs（工具引用字符串数组）、version。每次更新 SHALL 递增 version。所有查询与写入 SHALL 带 `class_id` 约束；跨班 id SHALL 返回 404 且与不存在同形；学生访问 SHALL 返回 403。

#### Scenario: SK1.1 教师新建并列出本班技能

- **GIVEN** 教师 A 属于班级 1，班级 2 已有名为「解题引导」的技能
- **WHEN** A 请求 `POST /api/skills` 提交 `{name:"解题引导", kind:"solve_hint", enabled:true, instruction:"先给思路", tool_refs:[]}` 后 `GET /api/skills`
- **THEN** 返回 200，列表中含该技能且 `version=1`，只含班级 1 的数据
- **AND** 班级 2 的技能不出现

#### Scenario: SK1.2 失败路径：学生与跨班

- **GIVEN** 学生 A1 与教师 B（班级 2）
- **WHEN** A1 请求 `POST /api/skills`，或 B 请求 `DELETE /api/skills/{班级1的技能id}`
- **THEN** 前者 403，后者 404（与删除不存在的 id 响应一致），均使用统一错误体

### Requirement: SK2 技能启用状态可观测地改变问答行为

系统 SHALL 在每次 `POST /api/ask` 时按发起者所在班加载 `enabled=true` 的技能，把其 instruction 叠加进生成提示词；解题引导类技能 SHALL 要求「先给思路、分步引导、不直接给最终答案」。当技能关闭或不存在时，提示词 SHALL 与未接入技能时一致。SSE 的 `meta` 事件 SHALL 带 `skill` 字段（name 与 enabled），使行为差异可核验。无模型网关的抽取式降级路径 SHALL 同样体现引导差异。

#### Scenario: SK2.1 同一问题开/关技能两次回答不同

- **GIVEN** 班级 1 本班材料命中问题「某题怎么做」，技能「解题引导」已启用
- **WHEN** 学生 A1 提问一次，教师关闭技能后 A1 再问同一问题
- **THEN** 两次 `meta.skill.enabled` 分别为 true 与 false
- **AND** 启用时的回答含分步引导且不含最终答案直给，关闭时回到既有回答形态

#### Scenario: SK2.2 无技能时提示词逐字不变

- **GIVEN** 班级 1 没有任何启用的技能
- **WHEN** 学生 A1 提问且材料命中
- **THEN** 生成使用的系统提示词等于 `qa.SystemPrompt()`，`meta.skill` 为 `{name:"", enabled:false}`
- **AND** 检索、引用与未命中判定与接入技能前一致

#### Scenario: SK2.3 失败路径：技能配置库故障不阻断问答

- **GIVEN** 技能查询依赖的数据库不可用，但材料检索正常
- **WHEN** 已登录学生提问
- **THEN** 问答主路径照常返回（按无技能处理），不返回 5xx
- **AND** 服务端留 ERROR 日志说明技能加载失败
