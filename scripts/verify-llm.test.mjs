import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createServer } from 'node:http';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import test from 'node:test';

const run = promisify(execFile);
const script = fileURLToPath(new URL('./verify-llm.mjs', import.meta.url));

for (const scenario of [
  { vectorStatus: 'available', vectorMatch: true, passes: true },
  { vectorStatus: 'unavailable', vectorMatch: false, passes: false },
  { vectorStatus: 'available', vectorMatch: false, passes: false },
]) {
  test(`模型验收：${scenario.vectorStatus}, vector_match=${scenario.vectorMatch}`, async (t) => {
    let material;
    const server = createServer(async (req, res) => {
      const json = (body, status = 200) => {
        res.writeHead(status, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(body));
      };
      switch (`${req.method} ${req.url}`) {
        case 'GET /health':
          return json({ llm: { enabled: true, chat_model: 'chat', embed_model: 'embed' } });
        case 'POST /api/login':
          return json({ access_token: 'test-token' });
        case 'POST /api/materials': {
          let body = '';
          for await (const chunk of req) body += chunk;
          material = JSON.parse(body);
          return json({ task_id: 'task', status_url: '/api/tasks/task' }, 202);
        }
        case 'GET /api/tasks/task':
          return json({ status: 'succeeded' });
        case 'GET /api/materials':
          return json({ materials: [{ id: 9, title: material.title, index_status: 'ready' }] });
        case 'POST /api/ask': {
          res.writeHead(200, { 'Content-Type': 'text/event-stream' });
          const token = material.title.match(/CampusClaw-LLM-\w+/)[0];
          const meta = {
            vector_retrieval: scenario.vectorStatus,
            citations: [{
              material_id: 9, material_title: material.title,
              snippet: material.content, vector_match: scenario.vectorMatch,
            }],
          };
          const events = [['meta', meta], ['delta', { text: token + ' [1]' }], ['done', {}]];
          return res.end(events.map(([event, data]) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`).join(''));
        }
        default:
          return json({}, 404);
      }
    });
    t.after(() => server.close());
    await new Promise((resolve, reject) => {
      server.once('error', reject);
      server.listen(0, '127.0.0.1', resolve);
    });
    const options = {
      env: {
        ...process.env,
        BASE_URL: `http://127.0.0.1:${server.address().port}`,
        TEACHER_USER: 'teacher-fixture',
        TEACHER_PASS: 'teacher-fixture',
        STUDENT_USER: 'student-fixture',
        STUDENT_PASS: 'student-fixture',
      },
      timeout: 10000,
    };
    if (scenario.passes) {
      const { stdout } = await run(process.execPath, [script], options);
      assert.match(stdout, /全部通过/);
    } else {
      await assert.rejects(run(process.execPath, [script], options), (err) => {
        assert.equal(err.code, 1);
        assert.match(err.stdout, /FAIL 本次材料的引用有向量召回/);
        if (scenario.vectorStatus !== 'available') assert.match(err.stdout, /FAIL 语义检索成功/);
        return true;
      });
    }
  });
}
