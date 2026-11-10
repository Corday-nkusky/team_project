#!/usr/bin/env node
// 双击「启动项目.cmd」后执行的入口：确保 Docker 引擎就绪 → 复用 scripts/up.mjs
// → 打开浏览器。所有交互都用中文，失败时给可直接照做的提示。
//
//   node scripts/launch.mjs           # 正常启动
//   node scripts/launch.mjs --check   # 只体检，不启动任何进程
import { spawn, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const url = "http://127.0.0.1:8080";
const checkOnly = process.argv.includes("--check");
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const dockerReady = () => {
  const r = spawnSync("docker", ["version", "--format", "{{.Server.Version}}"], {
    encoding: "utf8",
    windowsHide: true,
  });
  return r.status === 0 && (r.stdout || "").trim().length > 0;
};

const dockerDesktopExe = () => {
  const candidates = [
    path.join(process.env.ProgramFiles || "C:\\Program Files", "Docker", "Docker", "Docker Desktop.exe"),
    path.join(process.env["ProgramFiles(x86)"] || "C:\\Program Files (x86)", "Docker", "Docker", "Docker Desktop.exe"),
    path.join(process.env.LOCALAPPDATA || "", "Programs", "Docker Desktop", "Docker Desktop.exe"),
    path.join(process.env.LOCALAPPDATA || "", "Docker", "Docker Desktop.exe"),
  ];
  return candidates.find((p) => p && existsSync(p)) || "";
};

console.log("CampusClaw 一键启动");
console.log("----------------------------------------");

if (!existsSync(path.join(root, "deploy", "docker-compose.yml"))) {
  console.error("找不到 deploy/docker-compose.yml，请确认脚本放在项目根目录。");
  process.exit(1);
}

const exe = dockerDesktopExe();
if (checkOnly) {
  console.log(`Node.js        : ${process.version}`);
  console.log(`Docker 引擎    : ${dockerReady() ? "已就绪" : "未运行"}`);
  console.log(`Docker Desktop : ${exe || "未找到安装路径"}`);
  console.log(`应用地址       : ${url}`);
  console.log("体检完成，未启动任何进程。");
  process.exit(0);
}

if (!dockerReady()) {
  if (!exe) {
    console.error("Docker 引擎没运行，也没找到 Docker Desktop 的安装路径。");
    console.error("请先安装 Docker Desktop，或手动打开它之后重新双击本脚本。");
    process.exit(2);
  }
  console.log("Docker 引擎未运行，正在启动 Docker Desktop …");
  spawn(exe, [], { detached: true, stdio: "ignore", windowsHide: true }).unref();
  const deadline = Date.now() + 180_000;
  process.stdout.write("等待 Docker 引擎就绪");
  while (Date.now() < deadline) {
    if (dockerReady()) break;
    process.stdout.write(".");
    await sleep(3000);
  }
  process.stdout.write("\n");
  if (!dockerReady()) {
    console.error("等了 180 秒 Docker 引擎仍未就绪。请打开 Docker Desktop 看它是否在更新或报错，然后重新双击本脚本。");
    process.exit(3);
  }
  console.log("Docker 引擎已就绪。");
}

console.log("开始构建并启动容器（首次会拉镜像，可能要几分钟）…");
const up = spawnSync(process.execPath, [path.join(root, "scripts", "up.mjs")], {
  cwd: root,
  stdio: "inherit",
});
if (up.error || up.status !== 0) {
  console.error("启动失败。可手动排查：cd deploy && docker compose logs backend");
  process.exit(up.status || 4);
}

// 开浏览器这一步必须跨平台：macOS / Linux 上没有 cmd.exe，
// 若直接 spawn("cmd") 且不监听 error，Node 会抛 unhandled 'error' 并以退出码 1 结束，
// 让人误以为启动失败（其实容器已经起来了）。
const [openerCmd, openerArgs] =
  process.platform === "win32"
    ? ["cmd", ["/c", "start", "", url]]
    : process.platform === "darwin"
      ? ["open", [url]]
      : ["xdg-open", [url]];
const opened = spawn(openerCmd, openerArgs, {
  detached: true,
  stdio: "ignore",
  windowsHide: true,
});
opened.on("error", () => {
  console.log(`没能自动打开浏览器，请手动访问 ${url}`);
});
opened.unref();
console.log(`已就绪，浏览器打开 ${url}`);
console.log("演示账号见 README「演示账号」一节。");
