# 先红后绿门禁证据

> 课程第 8/9/11 课要求：至少保留一条判定为红的条目，当堂从红改到绿，留下修复前后证据（先红后绿输出连同提交号归档）。
> 本文件对应 `check-secrets.mjs`（也是 `make check-security` 的 SEC-05 密钥明文命中项）。

## 改坏的位置

临时新增被 git 跟踪的文件 `SEC-RED-TEST.txt`，内容为一条 pattern 匹配的假密钥：

```
test_placeholder = "sk-abcdefghijklmnopqrstuvwxyz0123456789"
```

（这是**构造的假值**，不是真实密钥；用 `git add -N` 让它进入 `git ls-files` 扫描范围。
`sk-` 后 38 位字母数字，命中 `check-secrets.mjs` 的 "OpenAI/DeepSeek 风格密钥" 规则。）

## 红屏：退出码非 0

命令：`node scripts/check-secrets.mjs`

```
✗ 密钥/配置卫生检查未通过：
  - SEC-RED-TEST.txt: 疑似真实密钥（OpenAI/DeepSeek 风格密钥）
```

退出码：**1**

## 修复内容

- 删除 `SEC-RED-TEST.txt`，并用 `git reset -q SEC-RED-TEST.txt` 撤销 intent-to-add。
- 不改动任何业务代码；门禁本身的规则未放宽。
- 提交号：本证据文件与红绿输出为同一笔未提交工作区改动（提交后在此填 hash）。

## 绿屏：退出码 0

命令：`node scripts/check-secrets.mjs`

```
✓ 密钥/配置卫生检查通过（扫描 166 个已跟踪文件）
```

退出码：**0**

复核命令（两条都应为 0）：

```
make check
make check-security
```

（本机 Windows Git Bash 未装 make，等价命令：`node scripts/make-check.mjs` 与
`node scripts/make-check.mjs security`，已实测全绿。）

## 观察

- 门禁在密钥明文出现在被跟踪文件时**确实会拦**（红屏证明），不是摆设。
- 红绿两次扫描的已跟踪文件数均为 166，说明变的只是文件内容，不是扫描口径。
- 该实验不产生任何真实密钥，结束后 `git status` 无残留（已核验）。
