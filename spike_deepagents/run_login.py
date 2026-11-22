"""Spike：用 Deep Agents 驱动 auto_ai 的 BrowserSession 跑一次登录场景。"""

import asyncio
import os
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))
os.environ.setdefault("NEO4J_ENABLED", "false")

from dotenv import dotenv_values
from langchain_core.tools import tool
from langchain_openai import ChatOpenAI

from deepagents import create_deep_agent

# 直接读 .env，避免系统环境变量里的同名 key 抢先（load_dotenv 默认不覆盖）。
CFG = dotenv_values(ROOT / ".env")

STEPS = {"count": 0}


async def main():
    from auto_ai.browser import browser_session

    async with browser_session(headless=True, cache_dir=ROOT / "cache") as session:
        @tool
        async def navigate(url: str) -> str:
            """打开页面并返回精简快照。第一步必须调用。"""
            STEPS["count"] += 1
            text, cached = await session.navigate(url)
            return f"[cached={cached}]\n{text}"

        @tool
        async def snapshot() -> str:
            """读取当前页面的精简快照，包含可交互元素的 role 与文字。"""
            STEPS["count"] += 1
            text, cached = await session.get_page_state()
            return f"[cached={cached}]\n{text}"

        @tool
        async def click(role: str, name: str) -> str:
            """点击元素。role 取按钮/输入框等的无障碍角色，name 取元素上的文字。"""
            STEPS["count"] += 1
            return await session.click_smart(role, name)

        @tool
        async def fill(role: str, name: str, value: str) -> str:
            """向输入框填入文本。"""
            STEPS["count"] += 1
            return await session.fill_smart(role, name, value)

        @tool
        async def wait_text(text: str) -> str:
            """等待页面出现指定文字。"""
            STEPS["count"] += 1
            ok = await session.wait_for(text)
            return f"found={ok}"

        @tool
        async def fill_password(role: str, name: str) -> str:
            """向密码输入框填入测试密码。密码由平台注入，调用方不需要也不应该知道明文。"""
            STEPS["count"] += 1
            result = await session.fill_smart(role, name, CFG["TEST_PASSWORD"])
            return "已填入密码" if "失败" not in result else result

        @tool
        async def current_url() -> str:
            """返回当前页面完整 URL。"""
            return session.get_current_url()

        @tool
        async def screenshot(name: str) -> str:
            """截图保存，返回文件路径。"""
            STEPS["count"] += 1
            return await session.screenshot(name)

        llm = ChatOpenAI(
            model=CFG.get("PLANNER_MODEL", "deepseek-chat"),
            api_key=CFG["OPENAI_API_KEY"],
            base_url=CFG.get("OPENAI_BASE_URL", "https://api.deepseek.com"),
            temperature=0,
        )

        agent = create_deep_agent(
            model=llm,
            tools=[navigate, snapshot, click, fill, fill_password, wait_text, current_url, screenshot],
            system_prompt=(
                "你是网页测试执行器。只能通过给定工具操作浏览器，不要写代码。\n"
                "流程：navigate 打开页面 -> snapshot 观察 -> click/fill 操作 -> "
                "每次页面变化后重新 snapshot 确认 -> 最后判断登录是否成功。\n"
                "遇到密码输入框必须用 fill_password，不要询问密码，也不要在回复里输出密码。\n"
                "不要猜测元素，只使用快照里真实出现的 role 与文字。\n"
                "结束时用一句话给出结论：登录成功，或失败原因。"
            ),
        )

        task = (
            f"打开 {CFG['TEST_BASE_URL']}，用账号 {CFG['TEST_USERNAME']} "
            "和密码完成登录，然后判断是否登录成功。"
        )

        started = time.time()
        result = await agent.ainvoke(
            {"messages": [{"role": "user", "content": task}]},
            config={"recursion_limit": 40},
        )
        elapsed = round(time.time() - started, 1)

        prompt_tokens = completion_tokens = 0
        rounds = 0
        for message in result["messages"]:
            usage = getattr(message, "usage_metadata", None) or {}
            prompt_tokens += usage.get("input_tokens", 0)
            completion_tokens += usage.get("output_tokens", 0)
            if type(message).__name__ == "AIMessage":
                rounds += 1

        print("\n===== Spike 结果 =====")
        print(f"耗时: {elapsed}s")
        print(f"模型轮次: {rounds}")
        print(f"浏览器操作次数: {STEPS['count']}")
        print(f"token: prompt={prompt_tokens} completion={completion_tokens}")
        print("最终回复:")
        print(result["messages"][-1].content)


if __name__ == "__main__":
    asyncio.run(main())
