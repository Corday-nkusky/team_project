"""POST /api/skills：教师可保存、学生 403、非法工具引用 400、开关可切换。"""
import sys, os, uuid
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../..")))
from common import check, request, login, section, TEACHER, STUDENT

section("POST /api/skills")
teacher, _ = login(*TEACHER)
student, _ = login(*STUDENT)
name = "引导-" + uuid.uuid4().hex[:8]
payload = {"name": name, "kind": "solve_hint", "enabled": True,
           "instruction": "先问学生卡在哪一步，再给一个提示。", "tool_refs": []}

code, body, raw = request("POST", "/api/skills", token=teacher, body=payload)
check("教师保存技能 200", code == 200, f"HTTP {code} {raw[:150]}")
check("返回版本号", bool(body and body.get("version", 0) >= 1))

code2, _, _ = request("POST", "/api/skills", token=student, body={**payload, "name": name + "x"})
check("学生保存被拒 403", code2 == 403, f"HTTP {code2}")

bad = {**payload, "name": name + "b", "tool_refs": ["badtool@none"]}
code3, b3, _ = request("POST", "/api/skills", token=teacher, body=bad)
check("非法工具引用 400", code3 == 400, f"HTTP {code3}")
check("错误码可读", bool(b3 and b3.get("error", {}).get("code") == "bad_tool_ref"))

# 关闭技能：同一名称再存一次 enabled=false，version 应递增。
code4, b4, _ = request("POST", "/api/skills", token=teacher, body={**payload, "enabled": False})
check("关闭技能保存成功", code4 == 200)
check("version 递增", bool(b4 and b4.get("version", 0) >= 2), f"version={b4 and b4.get('version')}")
