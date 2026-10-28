#!/usr/bin/env node
// 质量门禁：一条命令跑完「不能靠自觉」的检查。
//
//   node scripts/quality-gate.mjs                # 离线部分：分层 / 密钥 / 配置样例 / 格式 / 构建 / vet / 单测
//   node scripts/quality-gate.mjs --with-spec    # 追加 OpenSpec 严格校验（需 npx 网络）
//   node scripts/quality-gate.mjs --with-e2e     # 追加端到端验收（需服务与依赖已启动）
//
// Go 不在 PATH 时用 GO_BIN 指定，例如：
//   GO_BIN=/d/tools/go/bin/go.exe node scripts/quality-gate.mjs
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const backend = path.join(root, "backend");
const withSpec = process.argv.includes("--with-spec");
const withE2E = process.argv.includes("--with-e2e");

const goBin = process.env.GO_BIN || "go";
const gofmtBin =
  process.env.GOFMT_BIN ||
  (process.env.GO_BIN
    ? path.join(path.dirname(goBin), process.platform === "win32" ? "gofmt.exe" : "gofmt")
    : "gofmt");

const failures = [];

function run(name, cmd, args, { cwd = root, expectEmpty = false } = {}) {
  const started = Date.now();
  const res = spawnSync(cmd, args, { cwd, encoding: "utf8" });
  const ms = Date.now() - started;
  const output = `${res.stdout || ""}${res.stderr || ""}`.trim();
  const ok = !res.error && res.status === 0 && (!expectEmpty || output === "");
  if (ok) {
    console.log(`✓ ${name} (${ms}ms)`);
    return true;
  }
  console.error(`✗ ${name} (${ms}ms)`);
  if (res.error) console.error(`  ${res.error.message}`);
  if (output) console.error(output.split("\n").map((l) => `  ${l}`).join("\n"));
  if (expectEmpty && output) console.error("  以上文件未按 gofmt 格式化：gofmt -w .");
  failures.push(name);
  return false;
}

// --- 0. 配置样例完整性（不需要 Go）---
function checkConfigExample() {
  const examplePath = path.join(root, "deploy", ".env.example");
  const composePath = path.join(root, "deploy", "docker-compose.yml");
  const required = ["MYSQL_ROOT_PASSWORD", "MYSQL_DATABASE", "MYSQL_USER", "MYSQL_PASSWORD", "JWT_SECRET"];
  const problems = [];

  if (!existsSync(examplePath)) {
    problems.push("缺少 deploy/.env.example");
  } else {
    const example = readFileSync(examplePath, "utf8");
    const missing = required.filter((k) => !new RegExp(`^${k}=`, "m").test(example));
    if (missing.length) problems.push(`.env.example 缺少键：${missing.join(", ")}`);
  }
  if (!existsSync(composePath)) {
    problems.push("缺少 deploy/docker-compose.yml");
  } else if (!/\$\{JWT_SECRET:\?/.test(readFileSync(composePath, "utf8"))) {
    problems.push("compose 未把 JWT_SECRET 声明为必填（${JWT_SECRET:?…}）");
  }

  if (problems.length) {
    console.error("✗ 配置样例完整性");
    for (const p of problems) console.error(`  - ${p}`);
    failures.push("配置样例完整性");
    return false;
  }
  console.log("✓ 配置样例完整性");
  return true;
}

checkConfigExample();
run("分层依赖", process.execPath, ["scripts/check-layering.mjs"]);
run("密钥与配置卫生", process.execPath, ["scripts/check-secrets.mjs"]);

const goProbe = spawnSync(goBin, ["version"], { encoding: "utf8" });
if (goProbe.error || goProbe.status !== 0) {
  console.error("✗ 找不到 Go 工具链：请安装 Go，或用 GO_BIN 指向 go 可执行文件");
  failures.push("Go 工具链");
} else {
  run("gofmt", gofmtBin, ["-l", "."], { cwd: backend, expectEmpty: true });
  run("go build", goBin, ["build", "./..."], { cwd: backend });
  run("go vet", goBin, ["vet", "./..."], { cwd: backend });
  run("go test", goBin, ["test", "./..."], { cwd: backend });
}

if (withSpec) {
  run("OpenSpec 严格校验", "npx", ["--yes", "@fission-ai/openspec@latest", "validate", "--all", "--strict"]);
}

if (withE2E) {
  run("端到端验收（Go）", goBin, ["run", "./cmd/e2e"], { cwd: backend });
}

console.log("");
if (failures.length) {
  console.error(`质量门禁未通过：${failures.join("、")}`);
  process.exit(1);
}
console.log("质量门禁全部通过。");
