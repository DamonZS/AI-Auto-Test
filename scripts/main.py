"""
LangGraph + Playwright 自动化测试 入口

======================== 使用方式 ========================

## 1. 安装依赖
    pip install -r requirements.txt
    playwright install chromium

## 2. 配置 API Key
    cp .env.example .env
    # 编辑 .env，填入 DeepSeek 或 OpenAI 的 API Key

## 3. 运行测试
    python main.py --url <目标URL> --scenario "<测试场景>"

======================== 运行示例 ========================

# 基础登录测试
python main.py --url "https://example.com/login" --scenario "用 admin/123456 登录，验证跳转到首页"

# 搜索场景
python main.py --url "https://example.com" --scenario "在搜索框输入 iPhone，点击搜索，验证结果页包含商品列表"

# 表单提交
python main.py --url "https://example.com/contact" --scenario "填写留言：姓名张三、邮箱 test@qq.com、内容你好，提交后验证出现成功提示"

# 多页面跳转
python main.py --url "https://example.com" --scenario "从首页登录 admin/admin，进入个人中心，点击修改资料，修改昵称为新名称，验证保存成功"

# 显示浏览器窗口（调试模式）
python main.py --url "https://example.com/login" --scenario "测试登录" --no-headless

# 指定缓存目录
python main.py --url "https://example.com" --scenario "测试搜索" --cache-dir ./my_cache

======================== 目录结构 ========================

auto_ai/
├── main.py                 # CLI 入口（本文件）
├── config.py               # 配置（从 .env 读取）
├── agent.py                # LangGraph Agent（Planner/Executor/Reporter）
├── browser.py              # Playwright 浏览器封装
├── snapshot.py             # 页面快照 + 指纹 + 缓存
├── requirements.txt        # Python 依赖
├── .env.example            # 环境变量模板
├── cache/                  # 运行时生成：页面模板缓存
│   └── page_templates.json
├── screenshots/            # 运行时生成：测试截图
└── Neo4j 知识图谱          # 页面拓扑、元素覆盖、API关联（需 Neo4j 运行中）
"""

import asyncio
import argparse
from pathlib import Path

from auto_ai.browser import browser_session, BrowserSession
from auto_ai.agent import run_test


async def main():
    parser = argparse.ArgumentParser(
        description="LangGraph + Playwright 自动化测试 —— 用自然语言描述测试场景，AI 自动执行",
        epilog="""
示例:
  python main.py --url "https://example.com/login" --scenario "用 admin/123456 登录，验证跳转到首页"
  python main.py --url "https://example.com" --scenario "搜索 iPhone，验证搜索结果" --no-headless
        """,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--url", type=str, default="https://example.com", help="目标网站 URL")
    parser.add_argument("--scenario", type=str, required=True, help="测试场景，自然语言描述，例如：用 admin/123456 登录，验证跳转到首页")
    parser.add_argument("--headless", action="store_true", default=True, help="无头模式（默认开启，不显示浏览器窗口）")
    parser.add_argument("--no-headless", action="store_true", help="显示浏览器窗口，方便调试")
    parser.add_argument("--cache-dir", type=str, default="cache", help="页面模板缓存目录，默认 cache/")

    args = parser.parse_args()
    headless = not args.no_headless
    cache_dir = Path(args.cache_dir)

    print(f"目标 URL：{args.url}")
    print(f"测试场景：{args.scenario}")
    print(f"无头模式：{headless}")
    print(f"缓存目录：{cache_dir.resolve()}")

    async with browser_session(headless=headless, cache_dir=cache_dir) as session:
        report, passed = await run_test(session, args.url, args.scenario)

    # 关闭 Neo4j 连接
    try:
        from auto_ai.knowledge_graph import get_kg
        get_kg().close()
    except Exception:
        pass

    print(f"\n截图保存在 screenshots/ 目录")


if __name__ == "__main__":
    # Windows asyncio ProactorEventLoop 的 ProactorBasePipeTransport
    # 在垃圾回收时可能因事件循环已关闭而抛出 RuntimeError。
    # 这是 Python 在 Windows 上的已知行为，不影响任何功能，在此确定性压制。
    import asyncio.proactor_events
    _orig_del = asyncio.proactor_events._ProactorBasePipeTransport.__del__
    def _safe_del(self):
        try:
            _orig_del(self)
        except RuntimeError:
            pass
    asyncio.proactor_events._ProactorBasePipeTransport.__del__ = _safe_del

    asyncio.run(main())
