import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';

test('密钥门禁拒绝智谱形态和非占位配置，输出不回显值', (t) => {
  const root = mkdtempSync(path.join(tmpdir(), 'campusclaw-secret-gate-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, 'scripts'));
  mkdirSync(path.join(root, 'deploy'));
  const scanner = path.join(root, 'scripts', 'check-secrets.mjs');
  const example = path.join(root, 'deploy', '.env.example');
  const handoff = path.join(root, 'handoff.md');
  copyFileSync(new URL('./check-secrets.mjs', import.meta.url), scanner);
  writeFileSync(example, 'JWT_SECRET=replace-with-example\nLLM_API_KEY=<replace-me>\n');
  writeFileSync(handoff, '请通过受控渠道获取密钥。\n');
  execFileSync('git', ['init', '-q'], { cwd: root });
  execFileSync('git', ['add', 'deploy/.env.example', 'handoff.md'], { cwd: root });
  const scan = () => spawnSync(process.execPath, [scanner], { cwd: root, encoding: 'utf8' });
  const normal = scan();
  assert.equal(normal.status, 0, normal.stderr);

  const synthetic = ['a'.repeat(32), 'b'.repeat(16)].join('.');
  writeFileSync(handoff, '| LLM_API_KEY | ' + synthetic + ' |\n');
  const blocked = scan();
  assert.equal(blocked.status, 1);
  assert.match(blocked.stderr, /handoff\.md.*智谱 API Key/);
  assert.ok(!blocked.stderr.includes(synthetic));

  writeFileSync(handoff, '请通过受控渠道获取密钥。\n');
  writeFileSync(example, 'JWT_SECRET=replace-with-example\nLLM_API_KEY="synthetic-value"\n');
  const invalidExample = scan();
  assert.equal(invalidExample.status, 1);
  assert.match(invalidExample.stderr, /\.env\.example: LLM_API_KEY/);
  assert.ok(!invalidExample.stderr.includes('synthetic-value'));
});
