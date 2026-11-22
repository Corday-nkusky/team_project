#!/usr/bin/env node
// 真实网关全链路验证：/health → 教师上传（202 + 任务）→ 轮询索引 ready → 学生提问 → SSE 引用。
//
// 运行前需让本地后端启用模型网关（deploy/.env 中 LLM_* 四项齐全），并启动 8080：
//   node scripts/verify-llm.mjs
//
// 脚本只使用演示账号与随机测试材料，不读取或打印任何密钥。
import { randomBytes } from 'node:crypto';

const BASE = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const TEACHER = {
  username: process.env.TEACHER_USER || 'teacher1',
  password: process.env.TEACHER_PASS || 'teacher123',
};
const STUDENT = {
  username: process.env.STUDENT_USER || 'student1',
  password: process.env.STUDENT_PASS || 'student123',
};

let failures = 0;
function ok(name, cond, detail = '') {
  console.log(`${cond ? '  ok' : '  FAIL'} ${name}${detail ? ` · ${detail}` : ''}`);
  if (!cond) failures += 1;
}

async function request(path, { token, method = 'GET', body } = {}) {
  const res = await fetch(BASE + path, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(120000),
  });
  const text = await res.text();
  let json = null;
  try {
    json = JSON.parse(text);
  } catch {
    // 非 JSON 响应由调用方按状态码与原始文本判断
  }
  return { status: res.status, headers: res.headers, text, json };
}

async function login(user) {
  const res = await request('/api/login', { method: 'POST', body: user });
  if (res.status !== 200 || !res.json?.access_token) {
    throw new Error(`登录失败（${res.status}）：${res.text.slice(0, 200)}`);
  }
  return res.json.access_token;
}

function readSSE(text) {
  const events = [];
  for (const block of text.split(/\r?\n\r?\n/)) {
    if (!block.trim()) continue;
    let event = '';
    const data = [];
    for (const line of block.split(/\r?\n/)) {
      if (line.startsWith('event:')) event = line.slice(6).trim();
      else if (line.startsWith('data:')) data.push(line.slice(5).trim());
    }
    if (!event) continue;
    let payload = null;
    try {
      payload = JSON.parse(data.join('\n'));
    } catch {
      payload = null;
    }
    events.push({ event, payload });
  }
  return events;
}

async function ask(token, question) {
  const res = await fetch(BASE + '/api/ask', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ question }),
    signal: AbortSignal.timeout(120000),
  });
  const text = await res.text();
  return { status: res.status, contentType: res.headers.get('content-type') || '', text, events: readSSE(text) };
}

const unique = `CampusClaw-LLM-${randomBytes(8).toString('hex')}`;
const title = `LLM 验证材料 ${unique}`;
const content =
  `这是用于验证模型网关链路的材料。\n` +
  `验证词：${unique}。\n` +
  `材料中还说明：教师应在上传页提交讲义，学生只能查看本班材料。\n`;

console.log(`\n真实网关验证 · ${BASE}\n`);

try {
  const health = await request('/health');
  ok('GET /health 200', health.status === 200, `status=${health.status}`);
  ok('llm.enabled=true', health.json?.llm?.enabled === true, JSON.stringify(health.json?.llm || {}));
  ok('健康信息含 chat_model', Boolean(health.json?.llm?.chat_model), health.json?.llm?.chat_model || '');
  ok('健康信息含 embed_model', Boolean(health.json?.llm?.embed_model), health.json?.llm?.embed_model || '');

  const teacherToken = await login(TEACHER);
  const studentToken = await login(STUDENT);
  ok('教师与学生均可登录', Boolean(teacherToken && studentToken));

  const upload = await request('/api/materials', {
    token: teacherToken,
    method: 'POST',
    body: { title, content },
  });
  // 上传在第 8 课改为异步：202 + task_id，索引由 worker 完成，不再在请求内返回 ready。
  ok('上传返回 202', upload.status === 202, `status=${upload.status}`);
  const taskId = upload.json?.task_id;
  ok('响应含 task_id', typeof taskId === 'string' && taskId.length > 0, String(taskId ?? ''));
  ok('响应含 status_url', typeof upload.json?.status_url === 'string');
  if (!taskId) throw new Error('上传未返回 task_id，无法继续验证索引');

  const seen = [];
  let final = '';
  for (let i = 0; i < 240; i++) {
    const st = await request('/api/tasks/' + taskId, { token: teacherToken });
    const s = st.json?.status ?? `HTTP ${st.status}`;
    if (seen[seen.length - 1] !== s) seen.push(s);
    if (['succeeded', 'failed', 'dead'].includes(s)) {
      final = s;
      break;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  ok('索引任务到达 succeeded', final === 'succeeded', `终态=${final}，序列=${seen.join(' → ')}`);

  const list = await request('/api/materials', { token: teacherToken });
  const item = list.json?.materials?.find((m) => m.title === title);
  ok('列表可见新材料且为 ready', item?.index_status === 'ready', item ? `id=${item.id}` : 'not found');
  const materialId = item?.id;
  ok('拿到材料 id', Number.isInteger(materialId), String(materialId ?? ''));

  const q = `材料中的验证词是什么？请原样回答并标注引用。${unique}`;
  const stream = await ask(studentToken, q);
  ok('POST /api/ask 200', stream.status === 200, `status=${stream.status}`);
  ok('响应为 text/event-stream', stream.contentType.includes('text/event-stream'), stream.contentType);

  const meta = stream.events.find((e) => e.event === 'meta')?.payload || {};
  const deltas = stream.events.filter((e) => e.event === 'delta');
  const answer = deltas.map((e) => e.payload?.text || '').join('');
  const citations = Array.isArray(meta.citations) ? meta.citations : [];
  ok('未走 no_evidence', meta.no_evidence !== true);
  ok('语义检索成功', meta.vector_retrieval === 'available', meta.vector_retrieval || 'missing');
  ok('未出现 degraded', !stream.events.some((e) => e.event === 'degraded'));
  ok('未出现 error 事件', !stream.events.some((e) => e.event === 'error'));
  ok('收到流式增量', deltas.length > 0, `deltas=${deltas.length}`);
  ok('收到 done', stream.events.some((e) => e.event === 'done'));
  ok('回答非空', answer.trim().length > 0, `chars=${[...answer].length}`);
  ok('引用清单非空', citations.length > 0, `citations=${citations.length}`);
  ok(
    '引用指向本次材料',
    citations.some((c) => c.material_id === materialId && c.material_title === title),
    citations.map((c) => c.material_title).join(' | '),
  );
  ok('本次材料的引用有向量召回', citations.some((c) =>
    c.material_id === materialId && c.vector_match === true));
  ok(
    '引用片段含验证词',
    citations.some((c) => typeof c.snippet === 'string' && c.snippet.includes(unique)),
  );
  ok('回答原样包含验证词', answer.includes(unique));

} catch (err) {
  failures += 1;
  console.error(`\n验证异常：${err.message}`);
}

console.log(`\n结果：${failures === 0 ? '全部通过' : `${failures} 项失败`}\n`);
process.exit(failures === 0 ? 0 : 1);
