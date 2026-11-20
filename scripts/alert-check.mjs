#!/usr/bin/env node
// 本地真实告警链路：连续 N 次越阈后，向本机通知接收器 POST，收到 2xx 且 delivered=true 才算送达。
// 默认监听模式；--receiver 在 127.0.0.1 接收一条通知并写送达证据。
import { createServer } from "node:http";
import { mkdirSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function required(name) {
  const value = process.env[name];
  if (!value) throw new Error(`缺少必填环境变量 ${name}`);
  return value;
}

async function receiver() {
  const port = Number(process.env.ALERT_RECEIVER_PORT || 19099);
  const output = path.resolve(root, process.env.ALERT_DELIVERY_FILE || "reports/alert-delivery.json");
  const server = createServer((req, res) => {
    if (req.method !== "POST" || req.url !== "/notify") {
      res.writeHead(404).end("not found");
      return;
    }
    let raw = "";
    req.on("data", (chunk) => {
      raw += chunk;
      if (raw.length > 16384) req.destroy();
    });
    req.on("end", () => {
      try {
        const notification = JSON.parse(raw);
        const delivery = { delivered_at: new Date().toISOString(), notification };
        mkdirSync(path.dirname(output), { recursive: true });
        writeFileSync(output, JSON.stringify(delivery, null, 2) + "\n");
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ delivered: true }));
        console.log(`告警已送达并保存：${output}`);
        server.close();
      } catch (error) {
        res.writeHead(400).end(String(error));
      }
    });
  });
  await new Promise((resolve, reject) => server.listen(port, "127.0.0.1", resolve).once("error", reject));
  console.log(`本地通知接收器：http://127.0.0.1:${port}/notify`);
}

async function login(base) {
  const resp = await fetch(`${base}/api/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: required("ALERT_USERNAME"), password: required("ALERT_PASSWORD") }),
  });
  if (!resp.ok) throw new Error(`告警账号登录失败：HTTP ${resp.status}`);
  return (await resp.json()).access_token;
}

async function monitor() {
  const base = (process.env.BASE_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
  const notifyURL = required("ALERT_NOTIFY_URL");
  const queueThreshold = Number(process.env.ALERT_QUEUE_THRESHOLD || 0);
  const p95Threshold = Number(process.env.ALERT_P95_THRESHOLD_MS || 60000);
  const needed = Number(process.env.ALERT_CONSECUTIVE || 2);
  const interval = Number(process.env.ALERT_INTERVAL_MS || 2000);
  const attempts = Number(process.env.ALERT_ATTEMPTS || 10);
  const token = await login(base);
  let consecutive = 0;

  for (let attempt = 1; attempt <= attempts; attempt++) {
    const resp = await fetch(`${base}/api/observability`, { headers: { Authorization: `Bearer ${token}` } });
    if (!resp.ok) throw new Error(`读取观测失败：HTTP ${resp.status}`);
    const data = await resp.json();
    const slowest = Math.max(0, ...(data.requests || []).map((item) => Number(item.p95_ms) || 0));
    const breached = data.queue_depth > queueThreshold || slowest > p95Threshold;
    consecutive = breached ? consecutive + 1 : 0;
    console.log(`第 ${attempt} 次：queue_depth=${data.queue_depth} max_p95_ms=${slowest.toFixed(2)} consecutive=${consecutive}/${needed}`);
    if (consecutive >= needed) {
      const notification = {
        source: "campusclaw", metric: data.queue_depth > queueThreshold ? "queue_depth" : "http_p95_ms",
        value: data.queue_depth > queueThreshold ? data.queue_depth : slowest,
        threshold: data.queue_depth > queueThreshold ? queueThreshold : p95Threshold,
        consecutive, observed_at: new Date().toISOString(),
      };
      const delivered = await fetch(notifyURL, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(notification),
      });
      if (!delivered.ok) throw new Error(`通知接收器返回 HTTP ${delivered.status}`);
      const receipt = await delivered.json();
      if (receipt.delivered !== true) throw new Error("通知接收器未确认送达");
      console.log("告警真实触发并已送达。");
      return;
    }
    if (attempt < attempts) await sleep(interval);
  }
  console.log("未达到持续告警条件，未发送通知。");
}

try {
  if (process.argv.includes("--receiver")) {
    await receiver();
  } else {
    await monitor();
  }
} catch (error) {
  console.error(`告警失败：${error instanceof Error ? error.message : String(error)}`);
  process.exitCode = 1;
}
