#!/usr/bin/env python3
"""一键跑完 apitest/cases 下全部接口用例，生成 HTML 报告（不随作业提交）。

用法：
    python apitest/run_all.py
    BASE_URL=http://host:port python apitest/run_all.py

用例按 URL 组织：cases/api/login/POST.py 表示 POST /api/login。
每个用例脚本 import common 并通过 check() 登记断言；本脚本汇总结果、写报告，
以退出码判定：全绿 0，任一失败 1。报告默认写到 apitest/report.html。
"""
import os
import runpy
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import common


def discover():
    cases = []
    cases_dir = os.path.join(HERE, "cases")
    for root, _dirs, files in os.walk(cases_dir):
        for name in sorted(files):
            if name.endswith(".py"):
                cases.append(os.path.join(root, name))
    return sorted(cases)


def main():
    print(f"== CampusClaw 接口测试  target={common.BASE} ==\n")
    cases = discover()
    if not cases:
        print("未找到任何用例")
        return 1
    load_errors = []
    for path in cases:
        rel = os.path.relpath(path, HERE)
        print(f"\n--- {rel} ---")
        try:
            runpy.run_path(path, run_name="__apitest_case__")
        except SystemExit:
            pass
        except Exception as e:
            print(f"  ERROR 用例崩溃: {type(e).__name__}: {e}")
            load_errors.append(f"{rel}: {type(e).__name__}: {e}")
    for err in load_errors:
        common.check("用例执行无崩溃", False, err)

    report = os.environ.get("APITEST_REPORT", os.path.join(HERE, "report.html"))
    passed, total = common.write_report(report)
    print(f"\n== 结果：{passed}/{total} 通过 ==")
    print(f"报告：{report}")
    return 0 if passed == total else 1


if __name__ == "__main__":
    sys.exit(main())
