import asyncio
import importlib.util
import json
import os
import subprocess
import sys
import types
import unittest
from pathlib import Path
from unittest.mock import AsyncMock, patch


ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


def test_worker_import_without_model_credentials():
    environment = os.environ.copy()
    environment["PYTHON_DOTENV_DISABLED"] = "1"
    for credential in ("OPENAI_API_KEY", "OPENAI_ADMIN_KEY"):
        environment.pop(credential, None)
    launch = "import sys; sys.path.extend(" + json.dumps(sys.path) + "); "
    launch += "from agents.ui_runner import execute_task; assert 'auto_ai.agent' not in sys.modules"
    subprocess.run([sys.executable, "-c", launch], cwd=ROOT, env=environment, check=True, timeout=30)


class Tool:
    def __init__(self, func):
        self.func = func

    async def ainvoke(self, values):
        return await self.func(**values)


class FakeMessage:
    type = "ai"
    usage_metadata = {"input_tokens": 3, "output_tokens": 2}

    def __init__(self, content):
        self.content = content


class FakeSession:
    def __init__(self, assertion_results):
        self.assertion_results = iter(assertion_results)
        self.page = None
        self.actions = []

    async def __aenter__(self):
        return self

    async def __aexit__(self, *_args):
        return False

    async def navigate(self, _url):
        self.actions.append(("navigate", _url))
        return "page snapshot", False

    async def get_page_state(self, force_refresh=False):
        return "page snapshot", False

    async def click_smart(self, _role, _name):
        self.actions.append(("click", _role, _name))
        return "clicked"

    async def fill_smart(self, _role, _name, _value):
        self.actions.append(("fill", _role, _name, _value))
        return "filled"

    async def wait_for(self, _text):
        return next(self.assertion_results)

    def get_current_url(self):
        return "http://example.test"

    async def screenshot(self, name):
        return name


class FakePage:
    def __init__(self):
        self.handlers = {}

    def on(self, event, handler):
        self.handlers[event] = handler

    def emit(self, event, value=None):
        self.handlers[event](value)


def load_runner():
    spec = importlib.util.spec_from_file_location("ui_runner_under_test", ROOT / "agents" / "ui_runner.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ExecuteUITaskTests(unittest.TestCase):
    def setUp(self):
        self.tools = []

        def register_tool(func):
            registered = Tool(func)
            self.tools.append(registered)
            return registered

        fake_langchain_core = types.ModuleType("langchain_core")
        fake_tools = types.ModuleType("langchain_core.tools")
        fake_tools.tool = register_tool
        self.modules_patch = patch.dict(sys.modules, {
            "langchain_core": fake_langchain_core,
            "langchain_core.tools": fake_tools,
            "langchain_openai": types.SimpleNamespace(ChatOpenAI=lambda **_kwargs: object()),
            "deepagents": types.SimpleNamespace(create_deep_agent=lambda **_kwargs: None),
            "auto_ai.browser": types.SimpleNamespace(browser_session=lambda **_kwargs: None),
        })
        self.modules_patch.start()
        self.addCleanup(self.modules_patch.stop)
        self.runner = load_runner()

    def run_task(self, assertion_results, agent_result=None, agent_error=None, settings=None, session=None, scenario="test"):
        session = session or FakeSession(assertion_results)

        class BrowserSession:
            async def __aenter__(self):
                return session

            async def __aexit__(self, *_args):
                return False

        async def invoke(*_args, **_kwargs):
            if agent_error:
                raise agent_error
            tool_outputs = []
            for tool_call in agent_result or []:
                tool = next(t for t in self.tools if t.func.__name__ == tool_call[0])
                tool_outputs.append(await tool.ainvoke(tool_call[1]))
            return {"messages": [FakeMessage(output) for output in tool_outputs] + [FakeMessage("任务完成")]}

        browser_module = sys.modules["auto_ai.browser"]
        browser_module.browser_session = lambda **_kwargs: BrowserSession()
        fake_agent = types.SimpleNamespace(ainvoke=invoke)
        with patch.object(self.runner, "create_deep_agent", return_value=fake_agent):
            return asyncio.run(self.runner.execute_ui_task(
                {"base_url": "http://example.test", "scenario": scenario, "data": {"account": "private-account"}},
                settings or {"model": "m", "api_key": "k", "base_url": "http://llm.test", "max_steps": 5, "max_duration_seconds": 5},
            ))

    def test_passes_only_after_explicit_wait_text_assertion_succeeds(self):
        result = self.run_task([True], [("wait_text", {"text": "welcome"})])
        self.assertEqual(result["status"], "passed", result)
        self.assertTrue(result["passed"])
        self.assertEqual(result["assertions"], [{"text": "welcome", "passed": True}])

    def test_failed_wait_text_assertion_reports_failure(self):
        result = self.run_task([False], [("wait_text", {"text": "welcome"})])
        self.assertEqual(result["status"], "failed", result)
        self.assertFalse(result["passed"])
        self.assertEqual(result["failures"][0]["title"], "页面断言失败")

    def test_agent_text_without_assertion_needs_review(self):
        result = self.run_task([], [])
        self.assertEqual(result["status"], "needs_review")
        self.assertFalse(result["passed"])

    def test_exception_never_passes(self):
        result = self.run_task([], agent_error=RuntimeError("browser crashed"))
        self.assertEqual(result["status"], "error")
        self.assertFalse(result["passed"])

    def test_browser_startup_exception_never_passes(self):
        browser_module = sys.modules["auto_ai.browser"]
        browser_module.browser_session = lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("startup failed"))
        result = asyncio.run(self.runner.execute_ui_task(
            {"base_url": "http://example.test", "scenario": "test", "data": {}},
            {"model": "m", "api_key": "k", "base_url": "http://llm.test", "max_steps": 5, "max_duration_seconds": 5},
        ))
        self.assertEqual(result["status"], "error")
        self.assertFalse(result["passed"])

    def test_failed_result_includes_redacted_page_diagnostics(self):
        page = FakePage()
        session = FakeSession([False])
        session.page = page

        class Request:
            method = "POST"
            url = "https://example.test/api/login?token=secret-query"

            failure = "net::ERR_CONNECTION_RESET"

        class ResponseRequest:
            method = "POST"

        class Response:
            status = 500
            status_text = "server error"
            url = "https://example.test/api/login?token=secret-query"
            request = ResponseRequest()

        class PageError:
            message = "Bearer header-secret password=password-secret"

        async def emit_diagnostics(_text):
            page.emit("requestfailed", Request())
            page.emit("response", Response())
            page.emit("pageerror", PageError())
            return False

        session.wait_for = emit_diagnostics
        result = self.run_task([False], [("wait_text", {"text": "welcome"})], session=session)

        self.assertEqual(result["status"], "failed", result)
        self.assertEqual(1, len(result["failures"]))
        diagnostics = result["failures"][0]["diagnostics"]
        self.assertEqual([item["kind"] for item in diagnostics], ["request_failed", "http_error", "page_exception"])
        self.assertEqual(diagnostics[0]["data"]["text"], "net::ERR_CONNECTION_RESET")
        rendered = str(diagnostics)
        self.assertNotIn("header-secret", rendered)
        self.assertNotIn("password-secret", rendered)
        self.assertNotIn("token=secret-query", rendered)
        self.assertNotIn("Authorization", rendered)
        self.assertNotIn("body=", rendered)
        self.assertIn("HTTP 错误 1", result["report"])
        self.assertNotIn("header-secret", result["report"])

    def test_laya_choice_gates_the_next_browser_action(self):
        from agents import laya_decision_provider

        class Locator:
            async def count(self):
                return 1

            async def is_visible(self):
                return True

        class Provider:
            def __init__(self, *_args):
                pass

            async def choose(self, _state, criteria):
                choice = "button|登录"
                probabilities = {key: (0.92 if key == choice else 0.08 / (len(criteria) - 1)) for key in criteria}
                return {"choice": choice, "probabilities": probabilities, "margin": 0.84}

        session = FakeSession([True])
        session.page = FakePage()
        session.page.get_by_role = lambda *_args, **_kwargs: Locator()
        calls = [
            ("choose_control", {"intent": "登录", "candidates": {"button|登录": "登录按钮"}}),
            ("click", {"role": "button", "name": "登录"}),
            ("wait_text", {"text": "欢迎"}),
        ]
        with patch.object(laya_decision_provider, "LayaDecisionProvider", Provider):
            result = self.run_task(
                [True], calls, settings={
                    "model": "m", "api_key": "k", "base_url": "https://llm.test",
                    "max_steps": 5, "max_duration_seconds": 5, "laya_enabled": True,
                }, session=session,
            )
        self.assertEqual(result["status"], "passed", result)
        self.assertEqual(result["fallback_reason"], "")

    def test_structured_ui_steps_execute_in_order_and_hide_test_data(self):
        import json

        session = FakeSession([True])
        scenario = json.dumps({"steps": [
            {"action": "navigate", "url": "http://example.test/login"},
            {"action": "fill", "role": "textbox", "name": "Account", "data_key": "account"},
            {"action": "click", "role": "button", "name": "Sign in"},
            {"action": "wait_text", "text": "Welcome"},
        ]})
        result = self.run_task([], session=session, scenario=scenario)

        self.assertEqual(result["status"], "passed", result)
        self.assertEqual([action[0] for action in session.actions], ["navigate", "fill", "click"])
        self.assertEqual(session.actions[1][-1], "private-account")
        self.assertNotIn("private-account", result["report"])
        self.assertNotIn("private-account", str(result["failures"]))

    def test_screenshot_opt_in_requires_a_boolean(self):
        result = self.run_task([], scenario={"steps": [
            {"action": "assert_url", "expected": "example.test", "screenshot": "true"},
        ]})
        self.assertEqual(result["status"], "error")
        self.assertIn("screenshot", result["report"])

    def test_screenshot_failure_and_limits_leave_assertions_unchanged(self):
        session = FakeSession([])
        session.page = FakePage()
        session.page.frames = []
        session.page.screenshot = AsyncMock(side_effect=[
            RuntimeError("fixture-private-value"), *([b"x" * (128 * 1024 + 1)] * 3),
        ])
        scenario = {"steps": [
            {"action": "assert_url", "expected": "example.test", "screenshot": True} for _ in range(5)
        ]}
        result = self.run_task([], session=session, scenario=scenario)
        self.assertEqual(result["status"], "passed")
        self.assertEqual(session.page.screenshot.await_count, 4)
        self.assertEqual([step["screenshot_error"] for step in result["steps"]], [
            "unavailable", "too_large", "too_large", "too_large", "limit",
        ])
        self.assertTrue(all("screenshot_jpeg" not in step for step in result["steps"]))
        self.assertNotIn("fixture-private-value", str(result))


class LocalBrowserDiagnosticsTests(unittest.IsolatedAsyncioTestCase):
    async def test_opt_in_screenshots_capture_passed_and_failed_steps_with_masked_inputs(self):
        import time
        from agents.ui_runner import _execute_structured_steps
        from auto_ai.browser import browser_session
        from auto_ai.config import config

        with patch.object(config, "NEO4J_ENABLED", False):
            async with browser_session() as session:
                await session.page.set_content('''
                    <input aria-label="Account" style="position:absolute;left:10px;top:10px;width:200px;height:40px">
                    <iframe style="position:absolute;left:0;top:100px;border:0"
                      srcdoc="<textarea style='width:200px;height:40px'>fixture-private-note</textarea>"></iframe>
                ''')
                steps = [
                    {"action": "assert_url", "expected": "about:blank", "screenshot": False},
                    {"action": "fill", "role": "textbox", "name": "Account", "data_key": "account", "screenshot": True},
                    {"action": "assert_url", "expected": "missing", "screenshot": True},
                    {"action": "assert_url", "expected": "about:blank", "screenshot": True},
                ]
                result = await _execute_structured_steps(steps, {"account": "fixture-private-account"}, session, time.monotonic())
                evidence = result["steps"]
                self.assertEqual(result["status"], "failed")
                self.assertEqual([step["status"] for step in evidence], ["passed", "passed", "failed"])
                self.assertNotIn("screenshot_jpeg", evidence[0])
                for step in evidence[1:]:
                    pixels = await session.page.evaluate('''async (jpeg) => {
                        const image = new Image();
                        image.src = `data:image/jpeg;base64,${jpeg}`;
                        await image.decode();
                        const canvas = document.createElement('canvas');
                        canvas.width = image.width;
                        canvas.height = image.height;
                        const context = canvas.getContext('2d');
                        context.drawImage(image, 0, 0);
                        return [[100, 30], [100, 125], [500, 30]].map(([x, y]) =>
                            context.getImageData(x, y, 1, 1).data[0]);
                    }''', step["screenshot_jpeg"])
                    self.assertEqual(pixels, [0, 0, 255])
                self.assertEqual(await session.page.get_by_role("textbox", name="Account").input_value(), "fixture-private-account")

    async def test_structured_ui_controls_with_frame_row_and_open_shadow_root(self):
        from html import escape
        from urllib.parse import quote
        from agents.ui_runner import execute_ui_task
        from auto_ai.config import config

        form = """<table><tr><td>Unchanged</td><td><label>Ready <input type=checkbox></label></td></tr>
        <tr><td>Order 21</td><td><label>Ready <input type=checkbox></label>
        <label>Priority <select><option value=normal>Normal</option><option value=urgent>Urgent</option></select></label>
        <span id=result>Pending</span></td></tr></table><script>
        document.querySelector('select').addEventListener('change', (event) => {
          if (event.target.value === 'urgent' && document.querySelectorAll('input')[1].checked)
            document.querySelector('#result').textContent = 'Saved';
        });</script>"""
        page = f'''<iframe data-testid="orders" srcdoc="{escape(form, quote=True)}"></iframe>
        <div id="host"></div><script>document.querySelector('#host').attachShadow({{mode:'open'}}).innerHTML =
        `<button onclick="this.textContent='Done'">Finish</button>`;</script>'''
        steps = [
            {"action": "navigate", "url": "data:text/html," + quote(page)},
            {"action": "check", "frame": "orders", "row": "Order 21", "role": "checkbox", "name": "Ready"},
            {"action": "select", "frame": "orders", "row": "Order 21", "role": "combobox", "name": "Priority", "data_key": "priority"},
            {"action": "assert_text", "frame": "orders", "row": "Order 21", "text": "Saved"},
            {"action": "click", "role": "button", "name": "Finish"},
            {"action": "assert_text", "text": "Done"},
        ]
        with patch.object(config, "NEO4J_ENABLED", False):
            result = await execute_ui_task({"scenario": {"steps": steps}, "data": {"priority": "urgent"}}, {})
        self.assertEqual(result["status"], "passed", result)
        self.assertEqual([step["action"] for step in result["steps"]], [step["action"] for step in steps])
        self.assertTrue(all(step["duration_ms"] >= 0 for step in result["steps"]))
        self.assertNotIn("urgent", str(result))

    async def test_structured_failure_includes_real_browser_events(self):
        from agents.ui_runner import execute_ui_task
        from auto_ai.config import config

        async def handle(reader, writer):
            try:
                request = await reader.readuntil(b"\r\n\r\n")
                path = request.split(b" ", 2)[1]
                if path.startswith(b"/disconnect?"):
                    return
                if path.startswith(b"/unavailable?"):
                    status, body = b"503 Unavailable", b""
                else:
                    status = b"200 OK"
                    body = b"""<!doctype html><body><h1>Local fixture</h1>
                    <button>One</button><button>Two</button><button>Three</button>
                    <button>Four</button><button>Five</button>
                    <img src="/disconnect?token=fixture-query">
                    <img src="/unavailable?token=fixture-query">
                    <script>throw new Error(`Fixture http://fixture-user@${location.host}/detail`)</script>
                    </body>"""
                writer.write(
                    b"HTTP/1.1 " + status + b"\r\nContent-Type: text/html\r\nConnection: close\r\n"
                    + f"Content-Length: {len(body)}\r\n\r\n".encode() + body
                )
                await writer.drain()
            finally:
                writer.close()
                await writer.wait_closed()

        server = await asyncio.start_server(handle, "127.0.0.1", 0)
        url = f"http://127.0.0.1:{server.sockets[0].getsockname()[1]}"
        async with server:
            with patch.object(config, "NEO4J_ENABLED", False):
                result = await execute_ui_task({"scenario": {"steps": [
                    {"action": "navigate", "url": url},
                    {"action": "assert_url", "expected": "/missing", "match": "exact"},
                ]}}, {})

        self.assertEqual(result["status"], "failed", result)
        failure = result["failures"][0]
        self.assertEqual(failure["signature"], "ui_step_assert_url_failed")
        events = {event["kind"]: event["data"] for event in failure["diagnostics"]}
        self.assertEqual(events["request_failed"]["url"], url + "/disconnect")
        self.assertTrue(events["request_failed"]["text"])
        self.assertEqual(events["http_error"]["status"], 503)
        self.assertEqual(events["http_error"]["url"], url + "/unavailable")
        self.assertIn(url + "/detail", events["page_exception"]["text"])
        self.assertIn("请求失败 1", result["report"])
        self.assertIn("HTTP 错误 1", result["report"])
        for secret in ("fixture-query", "fixture-user"):
            self.assertNotIn(secret, str(result))


class TaskRoutingTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        fake_langchain_core = types.ModuleType("langchain_core")
        fake_tools = types.ModuleType("langchain_core.tools")
        fake_tools.tool = lambda func: func
        self.modules_patch = patch.dict(sys.modules, {
            "langchain_core": fake_langchain_core,
            "langchain_core.tools": fake_tools,
            "langchain_openai": types.SimpleNamespace(ChatOpenAI=lambda **_kwargs: object()),
            "deepagents": types.SimpleNamespace(create_deep_agent=lambda **_kwargs: None),
            "auto_ai.browser": types.SimpleNamespace(browser_session=lambda **_kwargs: None),
        })
        self.modules_patch.start()
        self.addCleanup(self.modules_patch.stop)

    async def test_routes_ui_task(self):
        runner = load_runner()
        with patch.object(runner, "execute_ui_task", new=AsyncMock(return_value={"status": "passed"})) as ui:
            result = await runner.execute_task({"type": "ui"}, {})
        ui.assert_awaited_once()
        self.assertEqual(result["status"], "passed")

    async def test_routes_api_task_to_api_runner(self):
        runner = load_runner()
        api_runner = types.SimpleNamespace(execute_api_task=AsyncMock(return_value={"status": "failed"}))
        with patch.dict(sys.modules, {"agents.api_runner": api_runner}), patch.object(runner, "execute_ui_task", new=AsyncMock()) as ui:
            result = await runner.execute_task({"type": "api"}, {"setting": True})
        api_runner.execute_api_task.assert_awaited_once_with({"type": "api"}, {"setting": True})
        ui.assert_not_awaited()
        self.assertEqual(result["status"], "failed")

    async def test_missing_api_runner_needs_review(self):
        runner = load_runner()
        with patch.dict(sys.modules, {"agents.api_runner": None}):
            result = await runner.execute_task({"type": "api"}, {})
        self.assertEqual(result["status"], "needs_review")
        self.assertFalse(result["passed"])

    async def test_routes_perf_task_to_perf_runner(self):
        runner = load_runner()
        perf_runner = types.SimpleNamespace(execute_perf_task=AsyncMock(return_value={"status": "failed"}))
        with patch.dict(sys.modules, {"agents.perf_runner": perf_runner}):
            result = await runner.execute_task({"type": "perf"}, {"setting": True})
        perf_runner.execute_perf_task.assert_awaited_once_with({"type": "perf"}, {"setting": True})
        self.assertEqual(result["status"], "failed")


if __name__ == "__main__":
    unittest.main()
