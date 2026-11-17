"""GET /api/skills：登录可读本班列表，未登录 401，学生可读。"""
import sys, os
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../..")))
from common import check, request, login, section, STUDENT

section("GET /api/skills")
code0, _, _ = request("GET", "/api/skills")
check("未登录 401", code0 == 401, f"HTTP {code0}")

student, _ = login(*STUDENT)
code, body, raw = request("GET", "/api/skills", token=student)
check("学生可读 200", code == 200, f"HTTP {code}")
check("返回 items 数组", isinstance(body, dict) and isinstance(body.get("items"), list))
