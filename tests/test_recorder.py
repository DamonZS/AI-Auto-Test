import asyncio
import json
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from io import StringIO
import os
from pathlib import Path
import subprocess
import sys
from threading import Thread

import pytest

from agents.recorder import _wait_for_terminal, record_ui_case


@pytest.fixture
def recording_site(tmp_path):
    (tmp_path / "index.html").write_text(
        """<meta charset="utf-8">
        <form action="/done.html">
          <label for="account">账号</label><input id="account" name="account">
          <label for="password">密码</label><input id="password" type="password">
          <span id="submit-name">保存草稿 "A"</span>
          <button aria-label="旧名称" aria-labelledby="submit-name"><span>保存</span></button>
        </form>
        <button id="unnamed"></button>""",
        encoding="utf-8",
    )
    (tmp_path / "done.html").write_text(
        '<meta charset="utf-8"><button aria-label="完成"><span>结束</span></button>', encoding="utf-8",
    )
    handler = partial(SimpleHTTPRequestHandler, directory=tmp_path)
    with ThreadingHTTPServer(("127.0.0.1", 0), handler) as server:
        thread = Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            yield f"http://127.0.0.1:{server.server_port}"
        finally:
            server.shutdown()
            thread.join()


async def test_chromium_records_named_actions_in_order_without_input_values(recording_site):
    async def interact(page):
        assert await page.context.cookies() == []
        await page.locator("#unnamed").click()
        await page.get_by_role("textbox", name="账号").press_sequentially("synthetic-account")
        await page.get_by_label("密码").fill("synthetic-password")
        await page.get_by_role("button", name='保存草稿 "A"').locator("span").click()
        await page.wait_for_url("**/done.html?*")
        await page.get_by_role("button", name="完成").click(button="right")
        await page.get_by_role("button", name="完成").click()
        return True

    draft = await record_ui_case(f"{recording_site}/?seed=synthetic-query#synthetic-fragment", interact, headless=True)

    assert draft == {"steps": [
        {"action": "navigate", "url": f"{recording_site}/"},
        {"action": "fill", "role": "textbox", "name": "账号", "data_key": "account"},
        {"action": "click", "role": "button", "name": '保存草稿 "A"'},
        {"action": "navigate", "url": f"{recording_site}/done.html"},
        {"action": "click", "role": "button", "name": "完成"},
    ]}
    assert "synthetic-" not in json.dumps(draft)


@pytest.mark.parametrize(("terminal_input", "save"), [("stop\n", True), ("quit\n", False), ("", False)])
async def test_chromium_terminal_commands(recording_site, monkeypatch, terminal_input, save):
    monkeypatch.setattr(sys, "stdin", StringIO(terminal_input))
    pages = []

    async def interact(page):
        pages.append(page)
        return await asyncio.wait_for(_wait_for_terminal(page), timeout=5)

    draft = await record_ui_case(recording_site, interact, headless=True)
    expected = {"steps": [{"action": "navigate", "url": f"{recording_site}/"}]} if save else None
    assert draft == expected
    assert pages[0].is_closed()


async def test_chromium_closing_tab_saves_while_stdin_is_open(recording_site, monkeypatch):
    async def interact(page):
        finished = asyncio.create_task(_wait_for_terminal(page))
        await page.close()
        return await asyncio.wait_for(finished, timeout=5)

    read_fd, write_fd = os.pipe()
    with os.fdopen(read_fd) as stdin, os.fdopen(write_fd, "w"):
        monkeypatch.setattr(sys, "stdin", stdin)
        draft = await record_ui_case(recording_site, interact, headless=True)

    assert draft == {"steps": [{"action": "navigate", "url": f"{recording_site}/"}]}


def test_record_ui_cli_rejects_invalid_url(tmp_path):
    script = Path(__file__).resolve().parents[1] / "scripts" / "case_cli.py"
    output = tmp_path / "draft.json"
    result = subprocess.run(
        [sys.executable, str(script), "record-ui", "file:///index.html", str(output)],
        capture_output=True, encoding="utf-8", timeout=10,
    )

    assert result.returncode == 2
    assert "HTTP(S)" in result.stderr
    assert result.stdout == ""
    assert not output.exists()
