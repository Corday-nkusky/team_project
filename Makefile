# CampusClaw 质量门禁入口（课程第 9/11 课要求 make check / make check-security）。
#
# 两个目标都以退出码判定：全绿 0，任一失败非 0。CI 跑的是同一条命令。
# Go 不在 PATH 时自动探测常见安装位置，也可用 GO_BIN 显式指定。

SHELL := /bin/sh

# Windows（Git Bash）与常见 Linux/macOS 的 Go 位置；优先级：环境变量 > 探测 > PATH。
GO_BIN ?= $(shell command -v go 2>/dev/null || \
	(ls /d/tools/go/bin/go.exe 2>/dev/null || \
	 ls /usr/local/go/bin/go 2>/dev/null || \
	 ls "$$HOME/go/bin/go" 2>/dev/null || echo go))

.PHONY: check check-security help

## check：分层 / 密钥 / 配置样例 / gofmt / build / vet / test（一条命令全跑）。
check:
	@echo "== make check =="
	GO_BIN="$(GO_BIN)" node scripts/quality-gate.mjs

## check-security：密钥扫描 + go vet + 全量 Go 单测（含 401/403/404 越权负向断言）。
## 依赖审计单独跑，见 reports/security-baseline.md；注入反例素材与判据见 reports/sec08-experiment.md。
check-security:
	@echo "== make check-security =="
	node scripts/check-secrets.mjs
	cd backend && "$(GO_BIN)" vet ./...
	cd backend && "$(GO_BIN)" test ./... -count=1
	@echo "check-security 全绿（密钥扫描 + vet + 全量单测，含越权负向用例）"

help:
	@echo "make check          全量质量门禁"
	@echo "make check-security 密钥 / 依赖 / 安全负向用例"
