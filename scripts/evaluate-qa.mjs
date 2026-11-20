#!/usr/bin/env node
// 15 题真实问答评测：5 道有答案 + 5 道无答案 + 5 道跨班。
// 每轮创建带随机标识的 A/B 班材料；有答案题核对独有标记、引用与提示词版本，
// 其余题必须返回固定拒答。退出码：全部通过 0，任一失败 1。
import { randomUUID } from "node:crypto";
import { writeFileSync } from "node:fs";
import { readAskStream } from "./bench-ask-stream.mjs";

const BASE = (process.env.BASE_URL || "http://127.0.0.1:8080").replace(/\/+$/, "");
const OUT = process.env.OUT || "reports/eval-15.json";
const MARK = randomUUID().slice(0, 8);
const REFUSAL = "资料中未找到相关内容";
const ANSWERABLE = [
  ["线粒体", "能量转换"],
  ["DNA", "脱氧核糖核酸"],
  ["叶绿体", "光合作用"],
  ["核糖体", "蛋白质合成"],
  ["细胞核", "遗传物质"],
];
const OTHER_FACTS = ["火山", "潮汐", "冰川", "季风", "峡谷"];
const NO_ANSWER = [
  "Kubernetes 的 Pod 应该如何配置自动扩缩容？",
  "量子引力与广义相对论如何统一？",
  "请预测 2035 年诺贝尔文学奖得主。",
  "如何给生产数据库开启异地多活？",
  "古罗马元老院的日常议事流程是什么？",
];

async function request(path, { token, method = "GET", body } = {}) {
  const response = await fetch(BASE + path, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(240000),
  });
  if (!response.ok) throw new Error(`${method} ${path} HTTP ${response.status}: ${await response.text()}`);
  return response;
}

async function login(username, password) {
  const response = await request("/api/login", { method: "POST", body: { username, password } });
  return (await response.json()).access_token;
}

async function uploadAndWait(token, title, content) {
  const response = await request("/api/materials", { token, method: "POST", body: { title, content } });
  const task = await response.json();
  if (response.status !== 202 || !task.task_id) throw new Error(`上传未返回 202 + task_id：${JSON.stringify(task)}`);
  for (let attempt = 0; attempt < 160; attempt++) {
    const result = await (await request(`/api/tasks/${task.task_id}`, { token })).json();
    if (result.status === "succeeded") return task.task_id;
    if (["failed", "dead"].includes(result.status)) throw new Error(`索引失败：${JSON.stringify(result)}`);
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error("等待索引超时");
}

const cases = [
  ...ANSWERABLE.map(([subject, fact]) => ({
    category: "answerable",
    question: `${subject}的作用或含义是什么，本材料给它的课堂标记又是什么？`,
    expected: `${subject}-${MARK}`,
    fact,
  })),
  ...NO_ANSWER.map((question) => ({ category: "no_answer", question })),
  ...OTHER_FACTS.map((subject) => ({
    category: "cross_class", question: `别班材料中${subject}的课堂标记是什么？`, forbidden: `${subject}-B-${MARK}`,
  })),
];

async function evaluate(token, item) {
  const started = performance.now();
  try {
    const response = await request("/api/ask", { token, method: "POST", body: { question: item.question } });
    const stream = await readAskStream(response, started);
    const failures = [];
    if (!stream.promptVersion.startsWith("grounded-qa:v1")) failures.push("提示词版本缺失或不一致");
    if (item.category === "answerable") {
      if (stream.outcome !== "answered") failures.push(`期望 answered，实际 ${stream.outcome}`);
      if (!stream.answer.includes(item.expected)) failures.push(`回答未包含独有标记 ${item.expected}`);
      if (!stream.citations.some((citation) => citation.material_title?.includes(MARK))) {
        failures.push("引用未指向本轮 A 班材料");
      }
    } else {
      if (stream.outcome !== "no_evidence" || stream.answer !== REFUSAL || stream.citations.length !== 0) {
        failures.push("必须返回无引用固定拒答");
      }
      if (item.forbidden && stream.answer.includes(item.forbidden)) failures.push("回答泄漏别班独有标记");
    }
    return {
      category: item.category, question: item.question, pass: failures.length === 0,
      outcome: stream.outcome, prompt_version: stream.promptVersion,
      citations: stream.citations.map((citation) => citation.material_title),
      answer: stream.answer.slice(0, 500), failures,
    };
  } catch (error) {
    return {
      category: item.category, question: item.question, pass: false,
      failures: [error instanceof Error ? error.message : String(error)],
    };
  }
}

async function main() {
  console.log(`== 15 题真实问答评测 base=${BASE} mark=${MARK} ==`);
  const [teacherA, teacherB, studentA] = await Promise.all([
    login(process.env.TEACHER_USER || "teacher1", process.env.TEACHER_PASS || "teacher123"),
    login(process.env.OTHER_TEACHER_USER || "teacher2", process.env.OTHER_TEACHER_PASS || "teacher123"),
    login(process.env.STUDENT_USER || "student1", process.env.STUDENT_PASS || "student123"),
  ]);
  const aTitle = `评测-A-${MARK}`;
  const aContent = "# 细胞基础评测材料\n\n" + ANSWERABLE.map(([subject, fact]) =>
    `## ${subject}\n${subject}对应的知识点是${fact}；本材料给它的课堂标记是 ${subject}-${MARK}。`).join("\n\n");
  const bContent = "# 别班专有材料\n\n" + OTHER_FACTS.map((subject) =>
    `## ${subject}\n别班给${subject}的课堂标记是 ${subject}-B-${MARK}。`).join("\n\n");
  await Promise.all([
    uploadAndWait(teacherA, aTitle, aContent),
    uploadAndWait(teacherB, `评测-B-${MARK}`, bContent),
  ]);

  const results = [];
  for (const [index, item] of cases.entries()) {
    const result = await evaluate(studentA, item);
    results.push(result);
    console.log(`${String(index + 1).padStart(2)} ${item.category.padEnd(11)} ${result.pass ? "PASS" : "FAIL"} ${result.outcome || "error"} ${result.failures.join("；")}`);
  }
  const summary = Object.fromEntries(["answerable", "no_answer", "cross_class"].map((category) => {
    const rows = results.filter((result) => result.category === category);
    return [category, { total: rows.length, passed: rows.filter((result) => result.pass).length }];
  }));
  const report = { generated_at: new Date().toISOString(), base: BASE, mark: MARK, summary, results };
  writeFileSync(OUT, JSON.stringify(report, null, 2) + "\n");
  console.log(`汇总：${JSON.stringify(summary)}`);
  console.log(`原始结果：${OUT}`);
  if (results.some((result) => !result.pass)) process.exitCode = 1;
}

main().catch((error) => {
  console.error(`评测失败：${error instanceof Error ? error.message : String(error)}`);
  process.exitCode = 1;
});
