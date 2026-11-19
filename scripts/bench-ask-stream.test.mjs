import test from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { once } from "node:events";
import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { readAskStream } from "./bench-ask-stream.mjs";

const frame = (name, data) => `event: ${name}\r\ndata: ${JSON.stringify(data)}\r\n\r\n`;
const meta = (hit) => frame("meta", { citations: hit ? [{ snippet: "原文" }] : [], no_evidence: !hit });
const delta = (text) => frame("delta", { text });
const done = frame("done", { conversation_id: null });
const answer = meta(true) + frame("degraded", { reason: "upstream_error" }) + delta("原文") + done;
const refused = meta(false) + delta("资料中未找到") + delta("相关内容") + done;
const headers = { "content-type": "text/event-stream; charset=utf-8" };

test("UTF-8 characters and frame boundaries may span chunks; first text precedes done", async () => {
  const bytes = new TextEncoder().encode(": keep-alive\r\n\r\n" + answer);
  const body = new ReadableStream({
    start(controller) {
      for (const byte of bytes.slice(0, -1)) controller.enqueue(Uint8Array.of(byte));
      setTimeout(() => { controller.enqueue(bytes.slice(-1)); controller.close(); }, 30);
    },
  });
  const sample = await readAskStream(new Response(body, { headers }), performance.now());
  assert.equal(sample.outcome, "answered");
  assert.ok(sample.firstTextMs >= 0 && sample.doneMs > sample.firstTextMs);
});

for (const [name, stream, reason] of [
  ["truncated", meta(true) + delta("部分"), "missing_done"],
  ["explicit error", meta(true) + delta("部分") + frame("error", { code: "interrupted" }), "stream_error"],
  ["missing citations", frame("meta", { citations: [], no_evidence: false }) + delta("回答") + done, "invalid_meta"],
  ["empty answer", meta(true) + delta("  ") + done, "empty_answer"],
  ["incorrect refusal", meta(false) + delta("猜测") + done, "invalid_refusal"],
]) {
  test(name, async () => {
    await assert.rejects(readAskStream(new Response(stream, { headers }), performance.now()), { message: reason });
  });
}

test("benchmark CLI counts outcomes, writes timings and fails on an HTTP 200 interruption", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "ask-bench-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const streams = [answer, refused, meta(true) + delta("部分")];
  const server = createServer((req, res) => {
    if (req.url === "/api/login") return res.end(JSON.stringify({ access_token: "test" }));
    assert.equal(req.url, "/api/ask");
    res.writeHead(200, headers).end(streams.shift());
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  async function run(requests) {
    const out = join(directory, "bench.json");
    const child = spawn(process.execPath, [fileURLToPath(new URL("./bench.mjs", import.meta.url))], {
      env: { ...process.env, BASE_URL: `http://127.0.0.1:${server.address().port}`, OUT: out,
        ENDPOINTS: "ask", LEVELS: "1", REQUESTS: String(requests) },
      stdio: ["ignore", "ignore", "inherit"],
    });
    const [code] = await once(child, "exit");
    const report = JSON.parse(await readFile(out, "utf8"));
    return [code, report.results.ask[0]];
  }

  const [green, success] = await run(2);
  assert.equal(green, 0);
  assert.deepEqual([success.answered, success.no_evidence, success.errors], [1, 1, 0]);
  assert.ok(success.first_text_p95_ms >= 0 && success.done_p95_ms >= success.first_text_p95_ms);
  const [red, failed] = await run(1);
  assert.equal(red, 1);
  assert.deepEqual([failed.answered, failed.no_evidence, failed.errors], [0, 0, 1]);
  assert.equal(failed.first_text_p95_ms, null);
  assert.equal(failed.done_p95_ms, null);
});
