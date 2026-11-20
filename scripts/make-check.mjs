#!/usr/bin/env node
// Makefile 两个目标的 Node 等价入口（Windows 未装 make 时使用）。
//
//   node scripts/make-check.mjs            等价于 make check
//   node scripts/make-check.mjs security   等价于 make check-security
//
// 与 Makefile 调用同一套门禁脚本，exit code 即判定，避免「本机没 make 就没法验证」。
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const security = process.argv.includes("security");
const goBin = process.env.GO_BIN || "go";

function run(name, cmd, args, cwd = root) {
  const res = spawnSync(cmd, args, { cwd, stdio: "inherit" });
  if (res.error || res.status !== 0) {
    console.error(`\n✗ ${name} 未通过`);
    process.exit(res.status || 1);
  }
}

if (security) {
  console.log("== check-security（Node 入口）==");
  run("安全门禁", process.execPath, ["scripts/security-gate.mjs"]);
} else {
  console.log("== check（Node 入口）==");
  run("质量门禁", process.execPath, ["scripts/quality-gate.mjs"]);
}
