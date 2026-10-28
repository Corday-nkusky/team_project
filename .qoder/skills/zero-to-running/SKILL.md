---
name: zero-to-running
description: 在全新环境把 CampusClaw 从零启动并验证：生成带随机密钥的 .env、docker compose 构建、等待 /health 就绪，或改为本地直接运行后端。适用于新成员上手、演示前准备、容器故障排查与验收复现。
---

# 从零到可运行

## 前置

- Docker Desktop（或 Docker Engine + Compose v2）已启动
- Node.js ≥ 20
- 可选：Go（仅为本地直接运行或跑 e2e 时需要）

## 一条命令

```bash
node scripts/up.mjs
```

它会：

1. 从 `deploy/.env.example` 生成 `deploy/.env`（若已存在则复用）
2. 把 `JWT_SECRET`、`MYSQL_PASSWORD`、`MYSQL_ROOT_PASSWORD` 的占位值替换为随机值
3. 执行 `docker compose up -d --build`
4. 轮询 `http://127.0.0.1:8080/health`，就绪后打印 `version`、`time`、`deps`

成功输出示例：

```
就绪：version=v0.0.1-skeleton time=2026-10-28T02:00:00Z deps={"mysql":"ok","qdrant":"ok"}
```

## 只准备配置

```bash
node scripts/up.mjs --prepare-only
```

用于 CI 或只想检查 `.env` 生成逻辑的场景。

## 本地直接运行（不用容器）

先确保 MySQL 与 Qdrant 可达，然后：

```bash
cd backend
export HTTP_ADDR=:8080 MYSQL_HOST=127.0.0.1 MYSQL_PORT=13306 \
       MYSQL_USER=campusclaw MYSQL_PASSWORD=<你的密码> MYSQL_DATABASE=campusclaw \
       QDRANT_URL=http://127.0.0.1:6333 JWT_SECRET=<至少32字符随机串>
GO_BIN=/d/tools/go/bin/go.exe go run ./cmd/server
```

缺任一必填项会**立即退出**并打印 JSON 日志（这是 spec R8 的预期行为，不是 bug）。

## 验证

```bash
curl -s http://127.0.0.1:8080/health
# 200 + status/version/time/deps

cd backend && GO_BIN=/d/tools/go/bin/go.exe go run ./cmd/e2e
# 41+ 项全绿，退出码 0
```

前端地址：`http://127.0.0.1:8080`；演示账号见 README。

## 常见问题

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| compose 报 `JWT_SECRET 未设置` | `.env` 缺失或值为占位符 | 跑 `node scripts/up.mjs` |
| 后端退出码 1，日志含 `JWT_SECRET 仍是示例值` | 手工复制了示例值 | 换成 `openssl rand -hex 32` |
| 3306 被占用 | 宿主机已有 MySQL | 本项目映射到 13306，无需改 |
| `/health` 503 | MySQL 或 Qdrant 未就绪 | `docker compose logs mysql qdrant backend` |
| e2e 连不上数据库 | 环境变量未指向 13306 | 显式导出 `MYSQL_PORT=13306` |

## 停止与清理

```bash
cd deploy && docker compose down          # 保留数据卷
cd deploy && docker compose down -v       # 连数据一起删（谨慎）
```
