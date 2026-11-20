## Why

问答压测只计 HTTP 状态，200 之后的流式错误与提前断流会被计入成功，完整响应时间也无法说明学生何时看到正文。

## What Changes

按问答事件契约核验命中与无依据拒答，分别统计首字、完成时间；增加脚本回归用例与 CI 检查。

## Impact

涉及 runtime-baseline 的压测验收、scripts/bench.mjs 与压测报告说明。
