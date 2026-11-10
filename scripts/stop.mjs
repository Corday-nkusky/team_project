#!/usr/bin/env node
// 双击「停止项目.cmd」后执行的入口：停掉 compose 里的四个容器。
// 与 scripts/up.mjs 对称：cwd 固定为 deploy/，让 docker compose 自动读取 deploy/.env。
//
//   node scripts/stop.mjs           # 停止并移除容器（数据卷保留）
//   node scripts/stop.mjs --check   # 只体检，列出容器，不停止任何东西
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const deployDir = path.join(root, "deploy");
const checkOnly = process.argv.includes("--check");

console.log("CampusClaw 一键停止");
console.log("----------------------------------------");

if (!existsSync(path.join(deployDir, "docker-compose.yml"))) {
  console.error("找不到 deploy/docker-compose.yml，请确认脚本放在项目根目录。");
  process.exit(1);
}

const docker = (args, opts = {}) =>
  spawnSync("docker", args, { cwd: deployDir, encoding: "utf8", windowsHide: true, ...opts });

// docker-compose.yml 里 MYSQL_* / JWT_SECRET 用了 ${VAR:?} 强制校验：
// 干净 clone 下来的仓库没有 deploy/.env（它被 git 忽略），直接 down 会被 compose 拒绝。
// 停止动作不需要真实配置，退回 .env.example 的占位值即可（只差几个非空字符串）。
const envPath = path.join(deployDir, ".env");
const envFile = existsSync(envPath) ? "" : path.join(deployDir, ".env.example");
if (envFile) {
  console.log("未找到 deploy/.env（本机还没启动过，或配置被删）。");
  console.log("改用 deploy/.env.example 的占位值执行停止，不影响已有数据卷。");
  console.log("");
}
const composeArgs = (...sub) =>
  envFile ? ["compose", "--env-file", envFile, ...sub] : ["compose", ...sub];

const engine = docker(["version", "--format", "{{.Server.Version}}"]);
if (engine.error || engine.status !== 0 || !(engine.stdout || "").trim()) {
  console.log("Docker 引擎没在运行，项目本来就没启动，无需停止。");
  process.exit(0);
}

if (checkOnly) {
  console.log("[体检] 当前项目容器：");
  const ps = docker(composeArgs("ps"), { stdio: "inherit" });
  console.log("体检完成，未停止任何容器。");
  process.exit(ps.status ?? 0);
}

console.log("正在停止并移除 mysql / qdrant / backend / worker …");
console.log("数据卷保留：材料、切片与向量索引不会丢；下次双击「启动项目.cmd」即可恢复。");
console.log("");

const down = docker(composeArgs("down"), { stdio: "inherit" });
if (down.error || down.status !== 0) {
  console.error("\n[FAILED] 停止失败，请把上面的报错内容发给我。");
  process.exit(down.status || 1);
}

console.log("\n[OK] 项目已停止。");
console.log("Docker Desktop 仍在后台运行；要彻底释放内存，请在托盘图标右键退出。");
process.exit(0);
