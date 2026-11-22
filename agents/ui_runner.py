"""Go 控制面 UI 执行端：领取任务，使用 Deep Agents + Playwright 执行并回报。"""

import argparse
import asyncio
import json
import os
import sys
import time
from pathlib import Path

import httpx
from langchain_core.tools import tool
from langchain_openai import ChatOpenAI
from deepagents import create_deep_agent

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from auto_ai.browser_diagnostics import BrowserDiagnostics, classify_events, redact_urls, safe_text


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", default="http://127.0.0.1:8090")
    parser.add_argument("--tenant", required=True)
    parser.add_argument("--poll", type=float, default=2)
    return parser.parse_args()


async def request(client, method, url, headers, **kwargs):
    response = await client.request(method, url, headers=headers, **kwargs)
    response.raise_for_status()
    return response.json()


async def execute_ui_task(task: dict, settings: dict) -> dict:
    started = time.monotonic()
    try:
        return await _execute_ui_task(task, settings)
    except Exception as exc:
        return {
            "status": "error",
            "passed": False,
            "needs_review": False,
            "failures": [],
            "duration_ms": int((time.monotonic() - started) * 1000),
            "report": f"UI 执行异常：{type(exc).__name__}: {safe_text(exc)}",
        }


async def _execute_ui_task(task: dict, settings: dict) -> dict:
    from auto_ai.browser import browser_session

    data = task.get("data") or {}
    steps = _structured_steps(task.get("scenario"))
    started = time.monotonic()

    usage = {"input": 0, "output": 0, "rounds": 0, "operations": 0}
    report = ""
    assertions = []
    status = "needs_review"
    failures = []

    async with browser_session(headless=True, cache_dir=ROOT / "cache") as session:
        diagnostics = BrowserDiagnostics(max_events=20, text_limit=200)
        if session.page is not None:
            diagnostics.bind_page(session.page)

        if steps is not None:
            result = await _execute_structured_steps(steps, task, data, session, started)
            return _with_browser_diagnostics(result, diagnostics)

        laya_provider = None
        fallback_reasons = []
        if settings.get("laya_enabled"):
            try:
                from agents.laya_decision_provider import LayaDecisionProvider

                laya_provider = LayaDecisionProvider(
                    settings["base_url"], settings["model"], settings["api_key"]
                )
            except Exception as exc:
                fallback_reasons.append(f"Laya 初始化失败：{type(exc).__name__}")

        pending_choice = None
        stop_after_choice = False

        @tool
        async def choose_control(intent: str, candidates: dict[str, str]) -> str:
            """Use Laya to resolve ambiguous visible controls. Candidate keys use role|accessible name."""
            nonlocal laya_provider, pending_choice, stop_after_choice
            if laya_provider is None:
                return "Laya 未启用或不可用，请使用确定性浏览器工具。"
            if not intent.strip() or not 1 <= len(candidates) <= 7:
                return "候选选择无效，保留确定性浏览器工具。"
            parsed = {}
            for key, description in candidates.items():
                parts = key.split("|", 1)
                if len(parts) != 2 or not all(part.strip() for part in parts) or not description.strip():
                    return "候选格式无效，候选键必须为 role|accessible name。"
                role, name = (part.strip() for part in parts)
                locator = session.page.get_by_role(role, name=name, exact=True)
                if await locator.count() != 1 or not await locator.is_visible():
                    return f"候选控件已变化或不唯一：{name}。请重新读取页面快照。"
                parsed[key] = (role, name)

            criteria = dict(candidates)
            criteria["STOP"] = "候选控件都不匹配当前操作，不执行。"
            try:
                state, _ = await session.get_page_state(force_refresh=True)
                decision = await laya_provider.choose(state + "\n操作意图：" + intent, criteria)
            except Exception as exc:
                laya_provider = None
                pending_choice = None
                stop_after_choice = False
                fallback_reasons.append(f"Laya 请求失败：{type(exc).__name__}")
                return "Laya 请求失败，已降级到确定性浏览器工具。请重新读取页面并按可见快照操作。"

            choice = decision["choice"]
            if choice == "STOP" or decision["probabilities"][choice] < 0.65 or decision["margin"] < 0.15:
                pending_choice = None
                stop_after_choice = True
                return "Laya 置信度不足或建议停止，不执行控件操作。"
            pending_choice = (choice, *parsed[choice])
            stop_after_choice = False
            return f"Laya 选择 {choice}，概率 {decision['probabilities'][choice]:.2f}。下一步只可执行该控件。"

        def choice_allows(role: str, name: str) -> bool:
            nonlocal pending_choice, stop_after_choice
            if stop_after_choice:
                return False
            if pending_choice is None:
                return True
            _, selected_role, selected_name = pending_choice
            if (role, name) != (selected_role, selected_name):
                return False
            pending_choice = None
            return True

        @tool
        async def navigate(url: str) -> str:
            """Navigate to the test page and return the current semantic snapshot."""
            usage["operations"] += 1
            snapshot, cached = await session.navigate(url)
            return f"[cached={cached}]\n{snapshot}"

        @tool
        async def snapshot() -> str:
            """Read the current page semantic snapshot after a page change."""
            usage["operations"] += 1
            text, cached = await session.get_page_state(force_refresh=True)
            return f"[cached={cached}]\n{text}"

        @tool
        async def click(role: str, name: str) -> str:
            """Click an element by its semantic role and accessible name."""
            if not choice_allows(role, name):
                return "控件与 Laya 决策不匹配或决策要求停止，未执行点击。"
            usage["operations"] += 1
            return await session.click_smart(role, name)

        @tool
        async def fill_data(role: str, name: str, data_key: str) -> str:
            """Fill an input with a value from the assigned test-data row without exposing the value."""
            if not choice_allows(role, name):
                return "控件与 Laya 决策不匹配或决策要求停止，未填写数据。"
            if data_key not in data:
                return f"测试数据字段不存在: {data_key}"
            usage["operations"] += 1
            result = await session.fill_smart(role, name, str(data[data_key]))
            return "已填入测试数据" if "失败" not in result else result

        @tool
        async def wait_text(text: str) -> str:
            """Wait for expected text to appear on the page."""
            usage["operations"] += 1
            passed = await session.wait_for(text)
            assertions.append({"text": redact_urls(text), "passed": bool(passed)})
            return f"found={bool(passed)}"

        @tool
        async def current_url() -> str:
            """Return the current page URL."""
            return session.get_current_url()

        @tool
        async def screenshot(name: str) -> str:
            """Capture a screenshot as execution evidence."""
            usage["operations"] += 1
            return await session.screenshot(name)

        llm = ChatOpenAI(
            model=settings["model"],
            api_key=settings["api_key"],
            base_url=settings["base_url"],
            temperature=0,
        )
        browser_tools = [navigate, snapshot, click, fill_data, wait_text, current_url, screenshot]
        if laya_provider is not None:
            browser_tools.append(choose_control)
        agent = create_deep_agent(
            model=llm,
            tools=browser_tools,
            system_prompt=(
                "你是受控 UI 测试执行器，只能使用提供的浏览器工具。"
                "先 navigate，再根据快照操作；页面变化后必须重新 snapshot。"
                "当同一操作存在多个合理可见控件时，若 choose_control 可用，先提交 role|accessible name 候选并遵从其结果；"
                "登录、表单输入必须使用 fill_data，data_key 只能使用测试数据字段名，"
                "不要输出字段值。操作成功必须通过页面文本、URL或状态可验证；不要把模型判断当成通过。"
                "失败时停止并说明失败步骤。"
            ),
        )
        prompt = (
            f"打开 {task['base_url']}，执行以下测试场景：{task['scenario']}。"
            f"可用测试数据字段：{', '.join(sorted(data)) or '无'}。"
        )
        try:
            result = await asyncio.wait_for(
                agent.ainvoke(
                    {"messages": [{"role": "user", "content": prompt}]},
                    config={"recursion_limit": settings["max_steps"]},
                ),
                timeout=settings["max_duration_seconds"],
            )
            messages = result.get("messages", [])
            for message in messages:
                meta = getattr(message, "usage_metadata", None) or {}
                usage["input"] += meta.get("input_tokens", 0)
                usage["output"] += meta.get("output_tokens", 0)
                if type(message).__name__ == "AIMessage":
                    usage["rounds"] += 1
            report = redact_urls(str(messages[-1].content if messages else "无 Agent 输出"))
            if assertions and all(assertion["passed"] for assertion in assertions):
                status = "passed"
            elif any(not assertion["passed"] for assertion in assertions):
                status = "failed"
                failures.append({
                    "signature": "ui_assertion_failed",
                    "title": "页面断言失败",
                    "evidence": str([item for item in assertions if not item["passed"]]),
                })
            else:
                report = f"未执行显式页面断言，需人工复核。\n{report}"
        except Exception as exc:
            status = "error"
            report = f"Agent 执行异常：{type(exc).__name__}: {safe_text(exc)}"

    return _with_browser_diagnostics({
        "status": status,
        "passed": status == "passed",
        "needs_review": status == "needs_review",
        "assertions": assertions,
        "failures": failures,
        "token_in": usage["input"],
        "token_out": usage["output"],
        "duration_ms": int((time.monotonic() - started) * 1000),
        "report": report,
        "fallback_reason": "；".join(fallback_reasons),
    }, diagnostics)


def _with_browser_diagnostics(result, diagnostics):
    if result["status"] == "passed" or not diagnostics.events:
        return result
    counts = classify_events(diagnostics.events)
    summary = (
        f"浏览器诊断：请求失败 {len(counts['request_failure'])}，"
        f"HTTP 错误 {len(counts['http_error'])}，"
        f"页面异常 {len(counts['page_exception'])}，崩溃 {len(counts['crash'])}。"
    )
    result["report"] = f"{result['report']}\n{summary}"
    if result["failures"]:
        result["failures"][0]["diagnostics"] = [
            {"kind": event.kind, "data": event.data} for event in diagnostics.events
        ]
    return result


def _structured_steps(scenario):
    if isinstance(scenario, str):
        try:
            scenario = json.loads(scenario)
        except json.JSONDecodeError:
            return None
    if not isinstance(scenario, dict) or "steps" not in scenario:
        return None
    steps = scenario["steps"]
    actions = {"navigate", "click", "fill", "wait_text", "assert_text", "assert_url"}
    if not isinstance(steps, list) or not steps:
        raise ValueError("结构化 UI 用例必须至少包含一个步骤")
    for step in steps:
        if not isinstance(step, dict) or step.get("action") not in actions:
            raise ValueError("结构化 UI 用例包含不支持的步骤")
        action = step["action"]
        required = {
            "navigate": ("url",),
            "click": ("role", "name"),
            "fill": ("role", "name", "data_key"),
            "wait_text": ("text",),
            "assert_text": ("text",),
            "assert_url": ("expected",),
        }[action]
        if any(not isinstance(step.get(key), str) or not step[key].strip() for key in required):
            raise ValueError(f"{action} 步骤缺少必填字段")
    return steps


async def _execute_structured_steps(steps, task, data, session, started):
    assertions = []
    results = []
    failures = []

    for index, step in enumerate(steps, start=1):
        action = step["action"]
        try:
            if action == "navigate":
                await session.navigate(step["url"])
            elif action == "click":
                result = await session.click_smart(step["role"], step["name"])
                if result.startswith("失败："):
                    raise RuntimeError(result)
            elif action == "fill":
                value = data[step["data_key"]]
                result = await session.fill_smart(step["role"], step["name"], str(value))
                if result.startswith("失败："):
                    raise RuntimeError(result)
            elif action in ("wait_text", "assert_text"):
                passed = await session.wait_for(step["text"])
                assertions.append({"step": index, "kind": action, "passed": passed})
                if not passed:
                    raise AssertionError(f"页面断言未满足：第 {index} 步")
            else:
                current_url = session.get_current_url()
                match = step.get("match", "contains")
                passed = step["expected"] in current_url if match == "contains" else step["expected"] == current_url
                assertions.append({"step": index, "kind": action, "passed": passed})
                if not passed:
                    raise AssertionError(f"URL 断言未满足：第 {index} 步")
            results.append({"step": index, "action": action, "status": "passed"})
        except Exception as exc:
            results.append({"step": index, "action": action, "status": "failed"})
            failures.append({
                "signature": f"ui_step_{action}_failed",
                "title": "UI 步骤执行失败",
                "evidence": f"第 {index} 步 {action}：{type(exc).__name__}: {safe_text(exc)}",
            })
            break

    passed_assertions = bool(assertions) and all(item["passed"] for item in assertions)
    status = "failed" if failures else "passed" if passed_assertions else "needs_review"
    return {
        "status": status,
        "passed": status == "passed",
        "needs_review": status == "needs_review",
        "assertions": assertions,
        "steps": results,
        "failures": failures,
        "token_in": 0,
        "token_out": 0,
        "duration_ms": int((time.monotonic() - started) * 1000),
        "report": f"结构化 UI 步骤执行 {len(results)}/{len(steps)}，断言 {len(assertions)} 项。",
        "fallback_reason": "",
    }


async def execute_task(task: dict, settings: dict) -> dict:
    task_type = task.get("type", "ui")
    if task_type == "ui":
        return await execute_ui_task(task, settings)
    if task_type == "api":
        try:
            from agents.api_runner import execute_api_task
        except ImportError as exc:
            return {
                "status": "needs_review",
                "passed": False,
                "needs_review": True,
                "report": f"接口执行器不可用，需人工复核：{exc}",
                "failures": [],
            }
        return await execute_api_task(task, settings)
    if task_type == "perf":
        try:
            from agents.perf_runner import execute_perf_task
        except ImportError as exc:
            return {
                "status": "needs_review",
                "passed": False,
                "needs_review": True,
                "report": f"性能执行器不可用，需人工复核：{exc}",
                "failures": [],
            }
        return await execute_perf_task(task, settings)
    return {
        "status": "needs_review",
        "passed": False,
        "needs_review": True,
        "report": f"不支持的测试类型：{task_type}，需人工复核。",
        "failures": [],
    }


async def run(args):
    agent_headers = {"X-Agent-Token": f"tenant:{args.tenant}"}
    async with httpx.AsyncClient(timeout=30) as client:
        settings = await request(client, "GET", f"{args.server}/api/agent/settings", agent_headers)
        if not settings.get("api_key"):
            raise RuntimeError("平台设置中没有配置 LLM API key")
        while True:
            task_response = await request(
                client, "POST", f"{args.server}/api/runs/heartbeat", agent_headers, json={}
            )
            task = task_response.get("run")
            if not task:
                await asyncio.sleep(args.poll)
                continue
            result = await execute_task(task, settings)
            await request(
                client,
                "POST",
                f"{args.server}/api/runs/{task['run_id']}/finish",
                agent_headers,
                json=result,
            )


if __name__ == "__main__":
    asyncio.run(run(parse_args()))
