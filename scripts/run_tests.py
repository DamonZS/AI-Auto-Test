"""auto_ai 测试运行入口 —— 一键生成 HTML + JUnit XML 报告

用法:
    python scripts/run_tests.py                   # 跑全部用例
    python scripts/run_tests.py -k login          # 只跑名称含 login 的用例
    python scripts/run_tests.py -k login --debug  # 调试模式（显示浏览器窗口）
"""

import argparse
import subprocess
import sys
from pathlib import Path
from datetime import datetime


def main():
    parser = argparse.ArgumentParser(
        description="auto_ai 自动化测试运行器 —— 一键跑用例 + 生成报告",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python scripts/run_tests.py
  python scripts/run_tests.py -k login
  python scripts/run_tests.py -k login --debug
        """,
    )
    parser.add_argument(
        "-k", "--filter", type=str, default="",
        help="按用例名称筛选（pytest -k 语法），例如: login, 搜索",
    )
    parser.add_argument(
        "--debug", action="store_true",
        help="调试模式，显示浏览器窗口（默认无头）",
    )

    args = parser.parse_args()

    # 确保目录存在
    reports_dir = Path("reports")
    screenshots_dir = Path("screenshots")
    reports_dir.mkdir(exist_ok=True)
    screenshots_dir.mkdir(exist_ok=True)

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    html_report = reports_dir / f"report_{timestamp}.html"
    xml_report = reports_dir / f"report_{timestamp}.xml"

    print("=" * 55)
    print("  auto_ai 自动化测试")
    print("=" * 55)
    print(f"  HTML 报告: {html_report}")
    print(f"  XML  报告: {xml_report}")
    if args.filter:
        print(f"  筛选条件:  {args.filter}")
    if args.debug:
        print(f"  调试模式:  显示浏览器窗口")
    print("=" * 55)

    # 构建 pytest 命令
    cmd = [
        sys.executable, "-m", "pytest",
        "tests/",
        "-v", "-s",
        f"--html={html_report}",
        "--self-contained-html",
        f"--junitxml={xml_report}",
    ]

    if args.filter:
        cmd.extend(["-k", args.filter])

    sys.exit(subprocess.run(cmd).returncode)


if __name__ == "__main__":
    main()
