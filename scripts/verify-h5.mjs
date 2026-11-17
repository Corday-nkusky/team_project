#!/usr/bin/env node
// 轻量端（H5）四步链路验证：登录 → 提交 202+task_id → 轮询 pending/running/succeeded → 跨班 404。
//
// 复用主应用同一套接口与认证；用 Node fetch 模拟手机浏览器的请求序列（spec TODO 9.6）。
// 只使用演示账号与随机标题，不读取或打印任何密钥。退出码即判定：全绿 0，任一失败 1。
//
//   node scripts/verify-h5.mjs
//   BASE_URL=http://host:8080 node scripts/verify-h5.mjs
import { randomUUID } from "node:crypto";

const BASE = (process.env.BASE_URL || "http://127.0.0.1:8080").replace(/\/+$/, "");
// 上传材料是教师权限（POST /api/materials 挂 requireRole("teacher")），
// 所以四步链路用教师账号；第 4 步跨班读取同样受班级隔离约束。
const USER = process.env.DEMO_USER || "teacher1";
const PASS = process.env.DEMO_PASS || "teacher123";

let fail = 0;
const ok = (name, cond, detail = "") => {
  console.log(`${cond ? "  ok" : "FAIL"} ${name}${detail ? ` · ${detail}` : ""}`);
  if (!cond) fail++;
};

async function req(path, { token, method = "GET", body } = {}) {
  const headers = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(BASE + path, {
    method, headers, body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(120000),
  });
  let json = null;
  try { json = await res.json(); } catch { /* SSE / 非 JSON */ }
  return { status: res.status, json };
}

console.log(`== H5 四步链路验证  base=${BASE} ==\n`);

// 第 1 步：登录取令牌。
const login = await req("/api/login", { method: "POST", body: { username: USER, password: PASS } });
ok("1 登录返回 200", login.status === 200, `HTTP ${login.status}`);
const token = login.json?.access_token;
ok("1 拿到 access_token", typeof token === "string" && token.length > 0);
if (!token) process.exit(1);

// 第 2 步：提交返回 202 + task_id。
const title = "H5 走查-" + randomUUID().slice(0, 8);
const submit = await req("/api/materials", {
  method: "POST", token,
  body: { title, content: "酶是活细胞产生的生物催化剂，化学本质主要是蛋白质。" },
});
ok("2 提交返回 202", submit.status === 202, `HTTP ${submit.status}`);
const taskId = submit.json?.task_id;
ok("2 返回 task_id", typeof taskId === "string" && taskId.length > 0, String(taskId));
ok("2 返回 status_url", typeof submit.json?.status_url === "string" && submit.json.status_url.includes(taskId ?? ""));
if (!taskId) process.exit(1);

// 第 3 步：轮询状态，记录出现过的状态序列。
const seen = [];
let final = "";
for (let i = 0; i < 120; i++) {
  const st = await req("/api/tasks/" + taskId, { token });
  const s = st.json?.status ?? ("HTTP " + st.status);
  if (seen[seen.length - 1] !== s) seen.push(s);
  if (["succeeded", "failed", "dead"].includes(s)) { final = s; break; }
  await new Promise((r) => setTimeout(r, 500));
}
ok("3 观察到 pending", seen.includes("pending"), seen.join(" → "));
ok("3 到达终态 succeeded", final === "succeeded", `终态=${final}，序列=${seen.join(" → ")}`);

// 第 4 步：换班查别班讲义。种子没有预置材料，先给 B 班上传一份带独有密语的，
// 再用 A 班令牌读它的 id —— 既验证 404，也验证响应不泄漏内容。
const otherLogin = await req("/api/login", {
  method: "POST", body: { username: "teacher2", password: "teacher123" },
});
ok("4 B 班教师登录", otherLogin.status === 200, `HTTP ${otherLogin.status}`);
const otherToken = otherLogin.json?.access_token;
if (!otherToken) process.exit(1);
const secret = "跨班密语-" + randomUUID().slice(0, 8);
const otherSubmit = await req("/api/materials", {
  method: "POST", token: otherToken,
  body: { title: "B班讲义-" + randomUUID().slice(0, 8), content: secret },
});
ok("4 B 班上传材料", otherSubmit.status === 202, `HTTP ${otherSubmit.status}`);
await new Promise((r) => setTimeout(r, 1500));
const otherList = await req("/api/materials", { token: otherToken });
const otherId = otherList.json?.materials?.[0]?.id;
ok("4 拿到 B 班材料 id", typeof otherId === "number", String(otherId));
if (typeof otherId !== "number") process.exit(1);
const other = await req("/api/materials/" + otherId, { token });
ok("4 跨班读取返回 404", other.status === 404, `HTTP ${other.status}`);
ok("4 404 使用统一错误体", other.json?.error?.code === "not_found", JSON.stringify(other.json?.error ?? {}));
ok("4 响应不含 B 班密语", !JSON.stringify(other.json ?? {}).includes(secret));

console.log(`\n结果：${fail === 0 ? "全绿" : fail + " 项失败"}`);
process.exit(fail === 0 ? 0 : 1);
