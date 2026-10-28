#!/usr/bin/env node
// 密钥与配置卫生门禁（spec R8 / security 规范）。
//
// 检查项：
//   1. 仓库不得跟踪 .env；本地密钥只存在 deploy/.env（git 忽略）。
//   2. 不得出现常见真实密钥形态（私钥、云厂商 key、GitHub/Slack token）。
//   3. compose 不得为 JWT_SECRET / MySQL 口令提供 `:-` 回落默认值。
//   4. deploy/.env.example 的 JWT_SECRET 必须是占位符或空值。
//   5. Go 代码不得为 JWT_SECRET 写死字面量默认值。
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const BINARY_EXT = new Set([
  ".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp", ".pdf", ".woff", ".woff2", ".ttf",
  ".zip", ".gz", ".tgz", ".mp4", ".mp3", ".jar", ".exe", ".dll", ".so", ".dylib", ".bin",
]);

const PATTERNS = [
  { name: "私钥文件", re: /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/ },
  { name: "OpenAI/DeepSeek 风格密钥", re: /\bsk-[A-Za-z0-9_-]{20,}\b/ },
  { name: "AWS Access Key", re: /\bAKIA[0-9A-Z]{16}\b/ },
  { name: "GitHub Token", re: /\bgh[pousr]_[A-Za-z0-9]{30,}\b/ },
  { name: "Slack Token", re: /\bxox[baprs]-[A-Za-z0-9-]{10,}\b/ },
  { name: "Google API Key", re: /\bAIza[0-9A-Za-z_-]{35}\b/ },
];

function trackedFiles() {
  try {
    return execFileSync("git", ["ls-files", "-z"], { cwd: root, encoding: "utf8" })
      .split("\0")
      .filter(Boolean);
  } catch {
    console.error("✗ 无法读取 git 索引；请在仓库根目录运行（需要 git）。");
    process.exit(1);
  }
}

const files = trackedFiles();
const violations = [];

for (const rel of files) {
  const base = path.basename(rel);
  if (base === ".env") {
    violations.push(`${rel}: .env 不得入库，密钥只保留在本机`);
    continue;
  }
  if (BINARY_EXT.has(path.extname(rel).toLowerCase())) continue;

  const full = path.join(root, rel);
  if (!existsSync(full) || statSync(full).size > 2 * 1024 * 1024) continue;
  const text = readFileSync(full, "utf8");

  for (const { name, re } of PATTERNS) {
    if (re.test(text)) violations.push(`${rel}: 疑似真实密钥（${name}）`);
  }
}

const composePath = path.join(root, "deploy", "docker-compose.yml");
if (existsSync(composePath)) {
  const compose = readFileSync(composePath, "utf8");
  for (const key of ["JWT_SECRET", "MYSQL_PASSWORD", "MYSQL_ROOT_PASSWORD"]) {
    const re = new RegExp(`${key}:\\s*\\$\\{[^}]*:-`);
    if (re.test(compose)) {
      violations.push(`deploy/docker-compose.yml: ${key} 不得提供 :- 回落默认值`);
    }
  }
}

const examplePath = path.join(root, "deploy", ".env.example");
if (existsSync(examplePath)) {
  const example = readFileSync(examplePath, "utf8");
  const m = example.match(/^JWT_SECRET=(.*)$/m);
  const value = m ? m[1].trim() : "";
  if (value && !/^(replace-with-|change-me)/.test(value)) {
    violations.push("deploy/.env.example: JWT_SECRET 必须是占位符或空值");
  }
}

const configGo = path.join(root, "backend", "internal", "config", "config.go");
if (existsSync(configGo)) {
  const src = readFileSync(configGo, "utf8");
  if (/env\(\s*"JWT_SECRET"\s*,\s*"/.test(src)) {
    violations.push("backend/internal/config/config.go: JWT_SECRET 不得有字面量默认值");
  }
}

if (violations.length) {
  console.error("✗ 密钥/配置卫生检查未通过：");
  for (const v of violations) console.error(`  - ${v}`);
  process.exit(1);
}

console.log(`✓ 密钥/配置卫生检查通过（扫描 ${files.length} 个已跟踪文件）`);
