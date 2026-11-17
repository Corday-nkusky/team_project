"""apitest 公共工具：请求封装、断言收集与 HTML 报告生成。

只依赖 Python 标准库。不读取、不打印任何密钥；令牌来自登录接口的返回值，
仅存在于进程内存。所有用例通过 BASE_URL 指向被测服务（默认本机 8080）。
"""

import json
import os
import time
import urllib.error
import urllib.request
from datetime import datetime

BASE = os.environ.get("BASE_URL", "http://127.0.0.1:8080").rstrip("/")
TEACHER = (os.environ.get("TEACHER_USER", "teacher1"), os.environ.get("TEACHER_PASS", "teacher123"))
STUDENT = (os.environ.get("STUDENT_USER", "student1"), os.environ.get("STUDENT_PASS", "student123"))

RESULTS = []


def request(method, path, token=None, body=None, timeout=30):
    """发一个 JSON 请求，返回 (status, dict|None, raw_text)。"""
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            code = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        code = e.code
    except Exception as e:  # 连接失败等
        return 0, None, str(e)
    parsed = None
    try:
        parsed = json.loads(raw)
    except (ValueError, TypeError):
        pass
    return code, parsed, raw


def login(username, password):
    code, body, _ = request("POST", "/api/login", body={"username": username, "password": password})
    if code != 200 or not body or "access_token" not in body:
        raise RuntimeError(f"登录失败: HTTP {code}")
    return body["access_token"], body.get("user", {})


def check(name, cond, detail=""):
    ok = bool(cond)
    RESULTS.append({"name": name, "ok": ok, "detail": str(detail)})
    print(("  PASS " if ok else "  FAIL ") + name + (f" · {detail}" if detail else ""))
    return ok


def section(title):
    print("\n== " + title + " ==")


def write_report(path):
    passed = sum(1 for r in RESULTS if r["ok"])
    total = len(RESULTS)
    rows = "\n".join(
        '<tr class="{cls}"><td>{name}</td><td>{status}</td><td>{detail}</td></tr>'.format(
            cls="pass" if r["ok"] else "fail",
            name=html(r["name"]), status="PASS" if r["ok"] else "FAIL", detail=html(r["detail"]),
        )
        for r in RESULTS
    )
    page = f"""<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<title>CampusClaw 接口测试报告</title>
<style>
body{{font-family:system-ui,"Microsoft YaHei",sans-serif;margin:2rem;color:#1f2937;background:#f9fafb}}
h1{{font-size:22px}} .summary{{font-size:15px;margin:1rem 0}}
table{{border-collapse:collapse;width:100%;background:#fff}}
th,td{{border:1px solid #e5e7eb;padding:8px 12px;text-align:left;font-size:13px}}
th{{background:#f3f4f6}} tr.pass td:nth-child(2){{color:#059669;font-weight:600}}
tr.fail td:nth-child(2){{color:#dc2626;font-weight:600}}
code{{background:#f3f4f6;padding:1px 4px;border-radius:3px}}
</style></head><body>
<h1>CampusClaw 接口测试报告</h1>
<p class="summary">生成时间：{datetime.now().isoformat(timespec='seconds')} · 通过 <strong>{passed}/{total}</strong> · 目标 <code>{html(BASE)}</code></p>
<table><thead><tr><th>用例</th><th>结果</th><th>说明</th></tr></thead><tbody>{rows}</tbody></table>
</body></html>"""
    with open(path, "w", encoding="utf-8") as f:
        f.write(page)
    return passed, total


def html(s):
    return (str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;"))
