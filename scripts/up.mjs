#!/usr/bin/env node
// 一条命令把 CampusClaw 拉起来：生成 .env（随机密钥）→ docker compose 构建启动 → 轮询 /health。
//
//   node scripts/up.mjs                  # 全流程
//   node scripts/up.mjs --prepare-only   # 只生成/修复 deploy/.env，不启动容器
//
// 设计约束（spec R8）：仓库不保存可用密钥；密钥只落在本机 deploy/.env（git 忽略）。
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { randomBytes } from "node:crypto";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const deployDir = path.join(root, "deploy");
const envPath = path.join(deployDir, ".env");
const examplePath = path.join(deployDir, ".env.example");
const prepareOnly = process.argv.includes("--prepare-only");

const readValue = (text, key) => {
  const m = text.match(new RegExp(`^${key}=(.*)$`, "m"));
  return m ? m[1].trim() : "";
};
const setValue = (text, key, value) => {
  const re = new RegExp(`^${key}=.*$`, "m");
  if (re.test(text)) return text.replace(re, `${key}=${value}`);
  return `${text.replace(/\s*$/, "")}\n${key}=${value}\n`;
};
const isPlaceholder = (v) =>
  !v || /change-me|changeme|replace-with|dev-insecure|example|your-/i.test(v);

if (!existsSync(examplePath)) {
  console.error("缺少 deploy/.env.example，无法生成配置");
  process.exit(1);
}

let env = existsSync(envPath) ? readFileSync(envPath, "utf8") : readFileSync(examplePath, "utf8");
const fresh = !existsSync(envPath);
const rotated = [];

// JWT 只影响本进程签发的令牌，任何情况下都可以安全轮换。
if (isPlaceholder(readValue(env, "JWT_SECRET"))) {
  env = setValue(env, "JWT_SECRET", randomBytes(32).toString("hex"));
  rotated.push("JWT_SECRET");
}

// MySQL 口令只在首次生成 .env 时随机化：已有数据卷的口令在初始化时写死，
// 事后改 .env 不会同步到库，反而会让后端连不上。
if (fresh) {
  for (const [key, bytes] of [
    ["MYSQL_PASSWORD", 18],
    ["MYSQL_ROOT_PASSWORD", 18],
  ]) {
    if (isPlaceholder(readValue(env, key))) {
      env = setValue(env, key, randomBytes(bytes).toString("hex"));
      rotated.push(key);
    }
  }
}

for (const [key, fallback] of [
  ["MYSQL_DATABASE", "campusclaw"],
  ["MYSQL_USER", "campusclaw"],
]) {
  if (!readValue(env, key)) env = setValue(env, key, fallback);
}

writeFileSync(envPath, env, { mode: 0o600 });
console.log(`已生成 ${path.relative(root, envPath)}`);
if (rotated.length) console.log(`已写入随机值：${rotated.join(", ")}`);

if (prepareOnly) process.exit(0);

const compose = spawnSync("docker", ["compose", "up", "-d", "--build"], {
  cwd: deployDir,
  stdio: "inherit",
});
if (compose.error || compose.status !== 0) {
  console.error("\n启动失败。请确认 Docker Desktop 已运行后重试：node scripts/up.mjs");
  process.exit(compose.status || 1);
}

const healthURL = process.env.HEALTH_URL || "http://127.0.0.1:8080/health";
const deadline = Date.now() + 120_000;
process.stdout.write("等待 /health 就绪");
while (Date.now() < deadline) {
  try {
    const resp = await fetch(healthURL);
    const body = await resp.json();
    if (resp.ok) {
      process.stdout.write("\n");
      console.log(`就绪：version=${body.version} time=${body.time} deps=${JSON.stringify(body.deps)}`);
      process.exit(0);
    }
  } catch {
    // 容器仍在启动，继续等待。
  }
  process.stdout.write(".");
  await new Promise((r) => setTimeout(r, 2000));
}
process.stdout.write("\n");
console.error("120 秒内 /health 未就绪，查看日志：cd deploy && docker compose logs backend");
process.exit(1);
