# 先红后绿门禁证据

> 课程第 8/9/11 课要求：至少保留一条判定为红的条目，当堂从红改到绿，留下修复前后证据（先红后绿输出连同提交号归档）。
> 本文件对应 `check-secrets.mjs`（也是 `make check-security` 的 SEC-05 密钥明文命中项）。

## 改坏的位置

临时新增被 git 跟踪的文件 `SEC-RED-TEST.txt`，假密钥由命令现场生成，**不在仓库保存字面量**——否则本报告自身会被扫描命中，门禁永远红：

```bash
python -c "open('SEC-RED-TEST.txt','w').write('test_placeholder = \"sk-' + 'a'*36 + 'b'*2 + '\"')"
git add -N SEC-RED-TEST.txt
```

写入的文件是 `sk-` 后接 38 位字母数字，命中 `check-secrets.mjs` 的 "OpenAI/DeepSeek 风格密钥" 规则。这是构造的假值，不是真实密钥；`git add -N` 只为让它进入 `git ls-files` 扫描范围。

## 红屏：退出码非 0

命令：`node scripts/check-secrets.mjs`

```
✗ 密钥/配置卫生检查未通过：
  - SEC-RED-TEST.txt: 疑似真实密钥（OpenAI/DeepSeek 风格密钥）
```

退出码：**1**（实测 2026-11-17 晚复跑）

## 修复内容

- 先 `git reset -q -- SEC-RED-TEST.txt` 撤销 intent-to-add，再删除 `SEC-RED-TEST.txt`。
- 不改动任何业务代码；门禁本身的规则未放宽。
- 提交定位：`git log --grep='修正三个验证脚本与令牌来源' --format='%h %s'`（本证据随该提交入库；自引用 hash 会随 amend 变化，故用可复核的提交信息定位）。

## 绿屏：退出码 0

命令：`node scripts/check-secrets.mjs`

```
✓ 密钥/配置卫生检查通过（扫描 225 个已跟踪文件）
```

退出码：**0**（实测 2026-11-17 晚复跑）

复核命令（两条都应为 0）：

```
make check
make check-security
```

（本机 Windows Git Bash 未装 make，等价命令：`node scripts/make-check.mjs` 与
`node scripts/make-check.mjs security`，已实测全绿。）

## 观察

- 门禁在密钥明文出现在被跟踪文件时**确实会拦**（红屏证明），不是摆设。
- 绿屏扫描 225 个已跟踪文件；红屏多出的 1 个是临时 intent-to-add 的 `SEC-RED-TEST.txt`，移除后回到 225，扫描口径未变。
- 该实验不产生任何真实密钥，结束后 `git status` 无残留（已核验）。
