"""GET /api/mcp/calls：教师可读、学生 403、非法 limit 400、令牌不出现在记录里。"""
import sys, os
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../../..")))
from common import check, request, login, section, TEACHER, STUDENT

section("GET /api/mcp/calls")
teacher, _ = login(*TEACHER)
student, _ = login(*STUDENT)

code, body, _ = request("GET", "/api/mcp/calls", token=teacher)
check("教师可读 200", code == 200, f"HTTP {code}")
check("返回 items 数组", isinstance(body, dict) and isinstance(body.get("items"), list))

code2, _, _ = request("GET", "/api/mcp/calls", token=student)
check("学生 403", code2 == 403, f"HTTP {code2}")

for q in ["limit=abc", "limit=0", "limit=-1"]:
    c, _, _ = request("GET", "/api/mcp/calls?" + q, token=teacher)
    check(f"非法 {q} 返回 400", c == 400, f"HTTP {c}")
