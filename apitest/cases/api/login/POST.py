"""POST /api/login：正确口令 200，错误口令 401，且响应不泄漏令牌以外的敏感字段。"""
import sys, os
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../..")))
from common import check, request, section

section("POST /api/login")
code, body, raw = request("POST", "/api/login", body={"username": "teacher1", "password": "teacher123"})
check("正确口令返回 200", code == 200, f"HTTP {code}")
check("响应含 access_token", bool(body and body.get("access_token")))
check("响应含 user 与班级", bool(body and body.get("user", {}).get("class_id")))

code2, body2, _ = request("POST", "/api/login", body={"username": "teacher1", "password": "wrong-password"})
check("错误口令返回 401", code2 == 401, f"HTTP {code2}")
check("错误口令不回令牌", not (body2 and "access_token" in body2))
