"""POST /api/mcp/servers：登记服务、令牌不回传、连接发现、断开后清单为空。"""
import sys, os, uuid
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../../..")))
from common import check, request, login, section, TEACHER

section("POST /api/mcp/servers（登记 / 连接 / 断开）")
teacher, _ = login(*TEACHER)
name = "tools-" + uuid.uuid4().hex[:8]
token = "apitest-token-" + uuid.uuid4().hex[:12]

code, body, raw = request("POST", "/api/mcp/servers", token=teacher, body={
    "name": name, "base_url": "http://mcp-server:8090", "auth_token": token,
})
check("登记服务 200", code == 200, f"HTTP {code} {raw[:150]}")
check("只回 token_configured 布尔", bool(body and body.get("token_configured") is True))
check("响应不含令牌明文", token not in raw and "auth_token" not in raw)
sid = body.get("id") if body else None
check("返回服务 id", bool(sid))

if sid:
    code2, b2, r2 = request("POST", f"/api/mcp/servers/{sid}/connect", token=teacher)
    check("连接成功 200", code2 == 200, f"HTTP {code2} {r2[:150]}")
    tools = (b2 or {}).get("tools", [])
    check("发现至少 2 个工具", len(tools) >= 2, f"tools={len(tools)}")
    if tools:
        check("工具三项齐全", all(t.get("name") and t.get("description") and t.get("inputSchema") for t in tools))

    # 重复连接：清单不翻倍。
    code3, b3, _ = request("POST", f"/api/mcp/servers/{sid}/connect", token=teacher)
    check("重复连接 200", code3 == 200)
    check("重复连接工具数不翻倍", len((b3 or {}).get("tools", [])) == len(tools))

    code4, b4, _ = request("GET", f"/api/mcp/servers/{sid}/tools", token=teacher)
    check("工具清单可读", code4 == 200 and len((b4 or {}).get("items", [])) == len(tools))

    code5, _, _ = request("POST", f"/api/mcp/servers/{sid}/disconnect", token=teacher)
    check("断开 200", code5 == 200)
    code6, b6, _ = request("GET", f"/api/mcp/servers/{sid}/tools", token=teacher)
    check("断开后清单为空", code6 == 200 and len((b6 or {}).get("items", [])) == 0)
