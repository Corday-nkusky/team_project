# 操作系统资料提交自审

变更：2026-11-19-add-os-lectures。此记录是作者自审，不能替代另一位成员评审。

- 接口入口：复用现有接口，无新路由。登录结果检查 role=teacher、class_id=1；模拟服务断言学生和跨班账号上传次数为零。
- 业务逻辑：OS1/OS2 对应 seed.test.mjs；六份数据校验、首次导入、重复跳过、冲突拒绝和索引失败均有断言。
- 数据库：脚本只调用 HTTP，不执行 SQL；材料及任务事务、唯一键和切片替换由已有服务负责。
- 公共函数：复用 biology 导入器而非复制上传和轮询逻辑；默认生物学数据路径保持不变。
- 错误处理：缺文件、摘要错误、错误身份、冲突、任务失败均非零退出；临时目录 finally 清理。中途失败不回滚已完成上传，重跑继续处理。
- 安全与隐私：仅正文、标题、文件名和摘要进入数据包；来源授权另存；令牌只在内存；不提交 deploy/.env、数据库备份和维护脚本。
- 性能：仅六份材料，串行上传并轮询，无额外数据库查询逻辑或新增查询索引。

## 实测

- `node seed-data/operating-systems/seed.mjs --check`：六份通过。
- `node --test seed-data/operating-systems/seed.test.mjs`：7/7 通过（Windows 宿主，沙箱外本地模拟服务）。
- `node seed-data/operating-systems/seed.mjs`：本机五服务运行中，已有 6 份 ready，同正文全部跳过，新受理 0、拒绝 0、失败 0。
- `node scripts/quality-gate.mjs`：Node 22 / Go 1.27 Linux 工具容器内配置、分层、密钥、gofmt、build、vet、test 全部通过；执行前 `npm --prefix frontend ci --no-audit --no-fund` 和 `npm --prefix frontend run build` 均成功。
- 同一 Linux 容器执行讲义测试 7/7 通过，验证跨平台换行归一和文件摘要。
- `npx --yes @fission-ai/openspec@latest validate 2026-11-19-add-os-lectures --strict`：valid，退出码 0。

当前结论：待另一位组员评审，未合并 main，未代签。合并前须完成 peer-review 清单与 OpenSpec 归档。
