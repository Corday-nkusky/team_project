#!/usr/bin/env node
// 三接口压测：提交（POST /api/materials）/ 查询（GET /api/materials）/ 检索（POST /api/ask）。
//
// 参数全部走环境变量，便于同脚本同参数做改进前后对比：
//   BASE_URL=http://127.0.0.1:8080
//   LEVELS=1,4,8              并发级别
//   REQUESTS=8                每个并发级别每接口发出的请求数
//   OUT=reports/bench-raw.json
//
// 提交用同一份内容（唯一约束复用同一任务，不产生大量材料）；检索走真实 SSE，
// 会消耗模型额度，请把 REQUESTS 控制在演示环境可承受范围内。
// 退出码：任一接口出现非 2xx 或问答流未正常完成 则 1，否则 0。
import { writeFileSync } from "node:fs";
import { readAskStream } from "./bench-ask-stream.mjs";
import { randomUUID } from "node:crypto";

const BASE = (process.env.BASE_URL || "http://127.0.0.1:8080").replace(/\/+$/, "");
const LEVELS = (process.env.LEVELS || "1,4,8").split(",").map((x) => Number(x.trim())).filter((x) => x > 0);
const REQUESTS = Math.max(1, Number(process.env.REQUESTS || 8));
const ENDPOINTS = (process.env.ENDPOINTS || "submit,list,ask").split(",").map((s) => s.trim()).filter(Boolean);
const OUT = process.env.OUT || "reports/bench-raw.json";
const TEACHER = {
  username: process.env.TEACHER_USER || "teacher1",
  password: process.env.TEACHER_PASS || "teacher123",
};
const STUDENT = {
  username: process.env.STUDENT_USER || "student1",
  password: process.env.STUDENT_PASS || "student123",
};
const MARK = "bench-" + randomUUID().slice(0, 8);
const ASK = "酶是什么？请依据本班材料回答。";

let bad = 0;
async function call(path, { token, method = "GET", body } = {}) {
  const started = performance.now();
  try {
    const res = await fetch(BASE + path, {
      method,
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(180000),
    });
    if (path === "/api/ask" && res.ok) {
      try {
        const stream = await readAskStream(res, started);
        return { ms: performance.now() - started, status: res.status, ...stream };
      } catch (error) {
        return { ms: performance.now() - started, status: res.status, error: error.message };
      }
    }
    await res.arrayBuffer();
    return { ms: performance.now() - started, status: res.status };
  } catch (e) {
    return { ms: performance.now() - started, status: 0, error: String(e) };
  }
}

async function login(user) {
  const r = await call("/api/login", { method: "POST", body: user });
  if (r.status !== 200) throw new Error(`登录失败 HTTP ${r.status}`);
  const res = await fetch(BASE + "/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(user),
  });
  return (await res.json()).access_token;
}

function percentile(sorted, fraction) {
  if (!sorted.length) return null;
  return Math.round(sorted[Math.floor((sorted.length - 1) * fraction)]);
}

function summarize(samples, wall) {
  const ms = samples.map((s) => s.ms).sort((a, b) => a - b);
  // 吞吐按墙钟时间算（并发请求的实际完成速率），不能用延迟之和，否则并发越高越被低估。
  return {
    requests: ms.length,
    errors: samples.filter((s) => s.status < 200 || s.status >= 300 || s.error).length,
    throughput_rps: wall > 0 ? +(ms.length / wall).toFixed(2) : 0,
    p50_ms: percentile(ms, 0.5),
    p95_ms: percentile(ms, 0.95),
    max_ms: Math.round(ms[ms.length - 1] ?? 0),
  };
}

async function runLevel(endpoint, level, token, student) {
  const samples = [];
  const perWorker = Math.ceil(REQUESTS / level);
  let next = 0;
  async function worker() {
    while (next < REQUESTS) {
      const i = next++;
      if (endpoint === "submit") {
        samples.push(await call("/api/materials", {
          token, method: "POST",
          body: { title: MARK, content: "压测用固定内容：酶是活细胞产生的生物催化剂。" },
        }));
      } else if (endpoint === "list") {
        samples.push(await call("/api/materials", { token }));
      } else {
        samples.push(await call("/api/ask", { token: student, method: "POST", body: { question: ASK } }));
      }
      void i;
    }
  }
  const started = performance.now();
  await Promise.all(Array.from({ length: level }, worker));
  const wall = (performance.now() - started) / 1000;
  const s = summarize(samples, wall);
  if (endpoint === "ask") {
    const completed = samples.filter((sample) => sample.outcome);
    s.answered = completed.filter((sample) => sample.outcome === "answered").length;
    s.no_evidence = completed.length - s.answered;
    for (const [metric, field] of [["first_text", "firstTextMs"], ["done", "doneMs"]]) {
      const times = completed.map((sample) => sample[field]).sort((a, b) => a - b);
      s[metric + "_p50_ms"] = percentile(times, 0.5);
      s[metric + "_p95_ms"] = percentile(times, 0.95);
    }
  }
  s.concurrency = level;
  s.wall_seconds = +wall.toFixed(2);
  bad += s.errors;
  return s;
}

console.log(`== 压测 base=${BASE} levels=${LEVELS.join(",")} requests/level=${REQUESTS} mark=${MARK} ==\n`);
const teacher = await login(TEACHER);
const student = await login(STUDENT);
const report = { base: BASE, levels: LEVELS, requests_per_level: REQUESTS, mark: MARK, generated_at: new Date().toISOString(), results: {} };

for (const endpoint of ENDPOINTS) {
  report.results[endpoint] = [];
  for (const level of LEVELS) {
    const s = await runLevel(endpoint, level, teacher, student);
    report.results[endpoint].push(s);
    if (endpoint === "ask") {
      console.log(`问答完成=${s.answered} 正常拒答=${s.no_evidence} 首字P95(ms)=${s.first_text_p95_ms ?? "—"} 完成P95(ms)=${s.done_p95_ms ?? "—"}`);
    }
    console.log(`${endpoint.padEnd(7)} C=${String(level).padStart(2)}  ${String(s.requests).padStart(2)} 次  ${String(s.throughput_rps).padStart(7)} req/s  p50=${String(s.p50_ms).padStart(5)}ms  p95=${String(s.p95_ms).padStart(5)}ms  errors=${s.errors}`);
  }
}

writeFileSync(OUT, JSON.stringify(report, null, 2));
console.log(`\n原始结果：${OUT}`);
console.log(`结果：${bad === 0 ? "全绿" : bad + " 个错误响应"}`);
process.exit(bad === 0 ? 0 : 1);
