#!/usr/bin/env node
// 分层依赖门禁（design D4）。
//
// 规则：
//   1. 只允许「同层或向下」依赖：web/config/auth/db/deps/llm/qdrant/retrieval/store(0)
//      ← qa(1) ← httpapi(2) ← cmd(3)；反向依赖一律失败。
//   2. httpapi 不得直接引用 llm / qdrant / retrieval：模型与向量访问必须经 qa。
//   3. httpapi 引用 store 仅限单表读写文件（下方白名单），业务编排不得落在 handler。
//   4. web 只允许标准库：静态资源包不得反向依赖任何内部包。
//   5. 新增内部包必须在本文件登记层级，否则失败——让分层决策显式发生。
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const backend = path.join(root, "backend");
const MODULE = "github.com/Corday-nkusky/team_project/backend";

const LAYERS = {
  "internal/config": 0,
  "internal/auth": 0,
  "internal/db": 0,
  "internal/deps": 0,
  "internal/llm": 0,
  "internal/qdrant": 0,
  "internal/retrieval": 0,
  "internal/store": 0,
  "internal/web": 0,
  "internal/qa": 1,
  "internal/httpapi": 2,
  "cmd/server": 3,
  "cmd/e2e": 3,
};

// httpapi 直接使用 store 的文件：均为「单表读写 / 会话记录」，不含检索与生成逻辑。
const HTTPAPI_STORE_ALLOWLIST = new Set(["auth.go", "materials.go", "ask.go", "router.go"]);
const HTTPAPI_FORBIDDEN_DIRECT = new Set(["internal/llm", "internal/qdrant", "internal/retrieval"]);

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === "dist" || entry.name === "node_modules" || entry.name.startsWith(".")) continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else if (entry.name.endsWith(".go")) out.push(full);
  }
  return out;
}

// parseImports 同时支持单行 import 与括号块，忽略块内注释行。
function parseImports(source) {
  const out = [];
  let inBlock = false;
  for (const raw of source.split(/\r?\n/)) {
    const line = raw.trim();
    if (!inBlock && /^import\s*\($/.test(line)) {
      inBlock = true;
      continue;
    }
    if (inBlock && line === ")") {
      inBlock = false;
      continue;
    }
    const m = inBlock
      ? line.match(/^(?:[\w.]+\s+)?"([^"]+)"/)
      : line.match(/^import\s+(?:[\w.]+\s+)?"([^"]+)"/);
    if (m) out.push(m[1]);
  }
  return out;
}

const violations = [];
let checked = 0;

for (const file of walk(backend)) {
  const pkgDir = path.relative(backend, path.dirname(file)).replaceAll("\\", "/");
  const fileName = path.basename(file);
  const relFile = path.relative(root, file).replaceAll("\\", "/");

  if (pkgDir === "internal" || pkgDir.startsWith("..")) continue;
  const sourceLayer = LAYERS[pkgDir];
  if (sourceLayer === undefined) {
    violations.push(`${relFile}: 包 ${pkgDir} 未在 scripts/check-layering.mjs 的 LAYERS 中登记`);
    continue;
  }

  for (const imp of parseImports(readFileSync(file, "utf8"))) {
    if (!imp.startsWith(`${MODULE}/`)) continue;
    const target = imp.slice(MODULE.length + 1);
    const targetLayer = LAYERS[target];
    checked++;
    if (targetLayer === undefined) {
      violations.push(`${relFile}: 引用了未登记的包 ${target}`);
      continue;
    }
    if (targetLayer > sourceLayer) {
      violations.push(`${relFile}: 反向依赖 ${pkgDir}(层${sourceLayer}) → ${target}(层${targetLayer})`);
    }
    if (pkgDir === "internal/httpapi" && HTTPAPI_FORBIDDEN_DIRECT.has(target)) {
      violations.push(`${relFile}: httpapi 不得直接引用 ${target}，应经 qa 间接调用`);
    }
    if (pkgDir === "internal/httpapi" && target === "internal/store" && !HTTPAPI_STORE_ALLOWLIST.has(fileName)) {
      violations.push(`${relFile}: httpapi 仅允许白名单文件直接使用 store`);
    }
    if (pkgDir === "internal/web") {
      violations.push(`${relFile}: web 只允许标准库，不得依赖 ${target}`);
    }
  }
}

if (violations.length) {
  console.error("✗ 分层依赖违规：");
  for (const v of violations) console.error(`  - ${v}`);
  console.error(`\n共 ${violations.length} 处。层级定义见 design D4 与 AGENTS.md「后端开发规范」。`);
  process.exit(1);
}

console.log(`✓ 分层依赖检查通过（${Object.keys(LAYERS).length} 个包，${checked} 条内部引用）`);
