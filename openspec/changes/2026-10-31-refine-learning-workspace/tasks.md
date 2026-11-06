# Tasks

> 完成一条勾一条，并在「实测」写入命令与结果。既有业务行为与 e2e 断言不得为重设计让路。

## 1. 规约与基线

- [x] 1.1 建立 change 四件套，记录视觉方向、兼容 selector、失败路径与 Non-goals
  - verify: `npx --yes @fission-ai/openspec@latest validate --all --strict` 退出码 0
  - 实测：2026-10-31 最终执行 `GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec --with-e2e`，其中 OpenSpec 严格校验通过并输出「质量门禁全部通过。」，退出码 0。

## 2. 应用外壳与登录

- [x] 2.1 建立视觉 token、内联图标、响应式应用外壳与唯一导航
  - verify: `npm run build` 通过；登录后角色、班级、用户、两个导航与退出入口可见
  - 实测：`frontend/` 下 `npm run build` 通过；`deploy/` 下 `docker compose up -d --build backend` 完成生产构建并重建容器，Vite 转换 36 个模块。真实 Chrome 验证角色、班级、用户、两个导航和退出入口均可见。
- [x] 2.2 重构登录页与演示账号交互，保留统一失败与忙碌状态
  - verify: 浏览器 e2e T1、T2、T2b、T8 通过；错误凭据仍显示统一错误
  - 实测：`BASE=http://127.0.0.1:8080 node scripts/e2e-ui.mjs` 的 T1、T2、T2b、T8 全部通过；登录页保留 `role=alert`、统一错误与忙碌禁用，演示账号只填表单、不自动登录。

## 3. 提问与引用工作台

- [x] 3.1 重构提问输入、提交/停止、等待与回答状态呈现
  - verify: 浏览器 e2e T4、T6、T11a–T11d 通过，空问题按钮禁用
  - 实测：生产 8080 浏览器 e2e 的 T4、T6、T11a–T11d 全部通过；空问题时提问按钮保持禁用，首字等待、停止、中断与降级状态均有对应断言。
- [x] 3.2 重构引用标记、来源与完整切片，支持键盘操作
  - verify: 浏览器 e2e T4b、T4c、T5、T5b 通过；Tab + Enter/Space 可切换引用来源
  - 实测：T4b、T4c、T5、T5b 通过；新增 T5c/T5d 使用 CDP 真实 Enter 与 Space 键输入，验证收起、展开、`:focus-visible` 和可见焦点样式，均通过。完整切片仍包含 300 字之后的独有尾注。
- [x] 3.3 修正提问页范围文案，避免暗示可指定单份材料
  - verify: 提问页不再出现「这份讲义」「对应讲义」；`cd frontend && npm run build` 退出码 0
  - 实测：`grep -rn "这份讲义\|对应讲义" frontend/src` 无匹配；`cd frontend && npm run build` 通过（36 modules），产物进入 `backend/internal/web/dist`。示例问题在演示语料上逐条实测：三羧酸循环、酶、有丝分裂/减数分裂、孟德尔定律均返回 200 且无 `no_evidence` / `degraded` / `error`；光合作用示例因回答「资料中未找到相关内容」未采用。

## 4. 材料管理工作台

- [x] 4.1 重构材料列表、角色空态、错误重试与状态统计
  - verify: 浏览器 e2e T10a–T10f 通过；学生不显示上传控件
  - 实测：T10a–T10f 全部通过；新增 T14 以学生真实登录并进入材料页，断言 `#t`、`#c`、`#f` 与「上传文本」按钮均不存在，通过。
- [x] 4.2 重构文本/文件上传、标题计数、通知与失败重索引
  - verify: 浏览器 e2e T3、T3b、T9、T9b 通过；上传失败显示可读错误
  - 实测：T3、T3b、T9、T9b、T10d/T10e 全部通过；真实文件选择框上传保留 multipart boundary，标题 255 字限制、可读错误与重试恢复均未回归。

## 5. 回归与视觉验收

- [x] 5.1 完成生产构建、质量门禁与两套 e2e
  - verify: `npm run build` 退出 0；`node scripts/quality-gate.mjs --with-spec --with-e2e` 退出 0；`node scripts/e2e-ui.mjs` 全部通过
  - 实测：生产镜像构建成功，`/health` 返回 200 且 MySQL / Qdrant 均为 `ok`。加载本机部署环境后，`GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs --with-spec --with-e2e` 的配置、分层、密钥、gofmt、build、vet、test、OpenSpec、Go e2e 全部通过；`BASE=http://127.0.0.1:8080 node scripts/e2e-ui.mjs` 最终为 36 通过、0 失败。
- [x] 5.2 检查 320、400、524、768、1440px 和减少动态效果，无横向溢出、无控制台错误、焦点可见
  - verify: 真实 Chrome 布局度量与截图通过；关键页面和状态均有桌面/窄屏证据
  - 实测：真实 Chrome 已检查登录、空提问、回答、材料页的桌面与窄屏截图；320、400、524、768、1440px 的提问页和材料页均无页面级横向溢出，控制台与页面异常为空。524px 实测表头 34.5px、状态徽标 54×22px、255 字标题行高 60.59375px；T5d 验证焦点可见，T13 验证减少动态效果媒体查询及实际样式。
- [x] 5.3 回填实测并同步必要的项目状态说明，不代签评审、不归档未评审 change
  - verify: 本文件每项实测可追溯；状态文档不再把本轮已验证项写成待复验
  - 实测：本文件已逐项回填命令与结果；`docs/project-status-guide.html` 已区分 main 基线与当前未提交工作区，并更新门禁、gofmt 根因修复及 36 项浏览器实测。未代签评审、未归档 change、未创建 Git 提交。
- [x] 5.4 修复浏览器 E2E 的跨平台 Chrome 路径与启动错误处理
  - verify: macOS 执行 `BASE=http://127.0.0.1:18080 node scripts/e2e-ui.mjs` 退出码 0；`CHROME=/不存在的路径 node scripts/e2e-ui.mjs` 退出码 1 且输出可诊断错误
  - 实测：2026-11-03 在 macOS 执行真实 Chrome E2E 全部通过；执行 `CHROME=/definitely/not/chrome node scripts/e2e-ui.mjs` 输出「无法启动 Chrome…ENOENT」、退出码 1，未再触发未处理的子进程错误。
- [ ] 5.5 修复阅读面板焦点迁移时序并稳定真实键盘驱动
  - verify: 真实 Chrome E2E 的 T3e、T3g、T5c、T5d 通过；完整脚本退出码 0
  - 实测：2026-11-03 在 macOS headless Chrome + CDP 实测为 47 通过、4 失败；失败项为 T3e、T3g、T5c、T5d。真实上传、LLM 流、引用、拒答与学生阅读均通过；焦点/键盘断言尚未闭环，不得勾选。
