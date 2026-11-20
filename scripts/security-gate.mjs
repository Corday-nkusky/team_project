#!/usr/bin/env node
// 安全门禁：Makefile、Windows Node 入口与 CI 共用同一判据。
// 任一命令非零即失败；依赖审计不得用“已登记”替代清零。
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const backend = path.join(root, "backend");
const frontend = path.join(root, "frontend");
const goBin = process.env.GO_BIN || "go";
const govulncheckBin = process.env.GOVULNCHECK_BIN || "";
const win = process.platform === "win32";
const failures = [];

function run(name, cmd, args, { cwd = root, shell = false } = {}) {
  console.log(`\n== ${name} ==`);
  const res = spawnSync(cmd, args, {
    cwd,
    shell,
    stdio: "inherit",
    env: { ...process.env, GOTOOLCHAIN: process.env.GOTOOLCHAIN || "auto" },
  });
  if (res.error || res.status !== 0) {
    if (res.error) console.error(res.error.message);
    failures.push(name);
    return false;
  }
  return true;
}

run("密钥与配置卫生", process.execPath, ["scripts/check-secrets.mjs"]);
run("Go vet", goBin, ["vet", "./..."], { cwd: backend });
run("Go 全量单测", goBin, ["test", "./...", "-count=1"], { cwd: backend });
run(
  "SQL/XSS/CSRF 常驻反例",
  goBin,
  ["test", "./internal/httpapi", "-run", "^TestSecurity", "-count=1", "-v"],
  { cwd: backend },
);
if (govulncheckBin) {
  run("Go 可达漏洞审计", govulncheckBin, ["./..."], { cwd: backend });
} else {
  // 固定扫描器版本，Go 工具链版本由 backend/go.mod 约束。
  run(
    "Go 可达漏洞审计",
    goBin,
    ["run", "golang.org/x/vuln/cmd/govulncheck@v1.10.0", "./..."],
    { cwd: backend },
  );
}
run(
  "npm 高危依赖审计",
  win ? "npm audit --audit-level=high --registry=https://registry.npmjs.org" : "npm",
  win ? [] : ["audit", "--audit-level=high", "--registry=https://registry.npmjs.org"],
  { cwd: frontend, shell: win },
);

if (failures.length) {
  console.error(`\n安全门禁未通过：${failures.join("、")}`);
  process.exit(1);
}
console.log("\n安全门禁全部通过：密钥、注入反例、Go/npm 依赖高危均为 0。");
