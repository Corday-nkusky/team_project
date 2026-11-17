"""端到端：连接自建工具服务 → 保存引用 formula_hint 的技能 → 提问 → 调用记录出现成功。

不依赖模型网关：无网关时按技能显式 tool_refs 执行一次（design D6 / spec MC3）。
核对 meta.tool_calls 与 GET /api/mcp/calls 里的 tool@server 一致（spec MC5.1）。
"""
import sys, os, uuid, json, time
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../..")))
from common import check, request, login, section, mcp_token, TEACHER, STUDENT, BASE
import urllib.request

section("端到端：技能 → 工具调用 → 记录")

teacher, _ = login(*TEACHER)
student, _ = login(*STUDENT)
suffix = uuid.uuid4().hex[:8]
server_name = "e2e-srv-" + suffix
skill_name = "e2e-skill-" + suffix
token = mcp_token()

# 1) 登记并连接自建服务。
code, srv, raw = request("POST", "/api/mcp/servers", token=teacher, body={
    "name": server_name, "base_url": "http://mcp-server:8090", "auth_token": token,
})
if code != 200 or not srv:
    check("前置：登记服务", False, f"HTTP {code} {raw[:200]}")
    raise SystemExit(0)
sid = srv["id"]
code, conn, raw = request("POST", f"/api/mcp/servers/{sid}/connect", token=teacher)
if code != 200:
    check("前置：连接服务", False, f"HTTP {code} {raw[:200]}（请确认 mcp-server 已启动）")
    raise SystemExit(0)
check("连接成功", conn.get("connected") is True)
ref = "formula_hint@" + server_name

# 2) 保存引用该工具的技能。
code, sk, raw = request("POST", "/api/skills", token=teacher, body={
    "name": skill_name, "kind": "solve_hint", "enabled": True,
    "instruction": "给一个分步线索，不直接给答案。", "tool_refs": [ref],
})
if code != 200:
    check("前置：保存技能", False, f"HTTP {code} {raw[:200]}")
    raise SystemExit(0)
check("技能保存成功且引用一致", ref in (sk or {}).get("tool_refs", []))

# 3) 提问并抓 SSE 首帧 meta。
question = "这道题该怎么入手？"  # 触发技能绑定工具，无需材料命中也能看到调用记录
meta = {}
events = []
req = urllib.request.Request(BASE + "/api/ask", data=json.dumps({"question": question}).encode(),
    headers={"Content-Type": "application/json", "Authorization": "Bearer " + student}, method="POST")
try:
    with urllib.request.urlopen(req, timeout=60) as resp:
        buf = ""
        ev = ""
        while True:
            chunk = resp.read(1)
            if not chunk:
                break
            buf += chunk.decode("utf-8", "replace")
            while "\n" in buf:
                line, buf = buf.split("\n", 1)
                line = line.rstrip("\r")
                if line.startswith("event:"):
                    ev = line[6:].strip()
                elif line.startswith("data:"):
                    data = line[5:].strip()
                    if ev == "meta":
                        try:
                            meta = json.loads(data)
                        except ValueError:
                            pass
                    events.append(ev)
                    ev = ""
except Exception as e:
    check("提问 SSE 可达", False, str(e))

check("收到 meta 事件", bool(meta), f"events={events[:5]}")
calls_in_meta = meta.get("tool_calls", [])
check("meta.tool_calls 含本次工具", any(c.get("tool_ref") == ref for c in calls_in_meta), str(calls_in_meta)[:200])
check("meta.skill 标记启用", (meta.get("skill") or {}).get("enabled") is True)

# 4) 核对调用记录：出现相同 tool@server。
code, body, raw = request("GET", "/api/mcp/calls?limit=50", token=teacher)
items = (body or {}).get("items", []) if body else []
match = [c for c in items if c.get("tool_ref") == ref]
check("记录中出现 tool@server", bool(match), f"匹配 {len(match)} 条")
if match:
    check("记录结果非空", match[0].get("result") in {"ok", "denied", "error"}, str(match[0].get("result")))
    check("记录含触发人与轮次", bool(match[0].get("actor_name")) and match[0].get("round", 0) >= 1)

# 5) 断开后引用即失效：再提问，meta 里应出现被拒。
request("POST", f"/api/mcp/servers/{sid}/disconnect", token=teacher)
req = urllib.request.Request(BASE + "/api/ask", data=json.dumps({"question": question}).encode(),
    headers={"Content-Type": "application/json", "Authorization": "Bearer " + student}, method="POST")
meta2 = {}
try:
    with urllib.request.urlopen(req, timeout=60) as resp:
        buf = ""
        ev = ""
        while True:
            chunk = resp.read(1)
            if not chunk:
                break
            buf += chunk.decode("utf-8", "replace")
            while "\n" in buf:
                line, buf = buf.split("\n", 1)
                line = line.rstrip("\r")
                if line.startswith("event:"):
                    ev = line[6:].strip()
                elif line.startswith("data:") and ev == "meta":
                    try:
                        meta2 = json.loads(line[5:].strip())
                    except ValueError:
                        pass
                    ev = ""
except Exception as e:
    check("断开后提问 SSE 可达", False, str(e))

denied = [c for c in meta2.get("tool_calls", []) if c.get("tool_ref") == ref and c.get("result") == "denied"]
check("断开后引用被拒", bool(denied), str(meta2.get("tool_calls", []))[:200])
