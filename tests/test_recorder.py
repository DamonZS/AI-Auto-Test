import asyncio
import json
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from io import StringIO
import os
from pathlib import Path
import subprocess
import sys
from threading import Event, Thread
from time import monotonic, sleep

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


@pytest.fixture(scope="module")
def recorder_qt():
    pytest.importorskip("PySide6.QtWebEngineWidgets")
    from PySide6.QtWidgets import QApplication

    return QApplication.instance() or QApplication(["Auto-Test"])


@pytest.fixture
def desktop_recording(recorder_qt, recording_site, monkeypatch, request):
    from PySide6.QtCore import QUrl
    from PySide6.QtGui import QIcon
    from PySide6.QtWebEngineWidgets import QWebEngineView
    from PySide6.QtWidgets import QToolBar

    monkeypatch.syspath_prepend(str(Path(__file__).resolve().parents[1] / "packaging"))
    import desktop_recorder
    from desktop_window import DesktopWindow

    browser_closed = []
    close_tab = getattr(request, "param", False)

    async def record(url, wait_for_stop, **options):
        async def interact(page):
            page.context.browser.on("disconnected", lambda _: browser_closed.append(True))
            await page.get_by_role("textbox", name="账号").fill("synthetic-account")
            await page.get_by_label("密码").fill("synthetic-password")
            await page.get_by_role("button", name='保存草稿 "A"').click()
            await page.wait_for_url("**/done.html?*")
            await page.get_by_role("button", name="完成").click()
            if close_tab:
                await page.close()
            return await wait_for_stop(page)

        return await record_ui_case(
            url, interact, headless=os.getenv("AUTOTEST_RECORDER_HEADED_TEST") != "1", **options,
        )

    monkeypatch.setattr(desktop_recorder, "record_ui_case", record)
    recorder_qt.setQuitOnLastWindowClosed(False)
    window = DesktopWindow(recorder_qt, QIcon(), QUrl(recording_site))
    try:
        assert not window.findChildren(QToolBar)
        page = window.findChild(QWebEngineView).page()
        loaded = []
        page.loadFinished.connect(loaded.append)
        wait_for_recorder(lambda: loaded)
        assert loaded == [True]
        page.runJavaScript("""
            if (window.caseWorkbook) {
                window.caseWorkbook.recordUiCase();
            } else {
                window.addEventListener('case-workbook-ready', () => window.caseWorkbook.recordUiCase(), { once: true });
            }
        """)
        wait_for_recorder(window.recorder.isVisible)
        window.recorder.address.setText(f"{recording_site}/?seed=synthetic-query")
        yield window, browser_closed
    finally:
        window.recorder.shutdown()
        window.tray.hide()
        window.hide()


def wait_for_recorder(predicate):
    from PySide6.QtWidgets import QApplication

    deadline = monotonic() + 15
    while not predicate():
        assert monotonic() < deadline, "Qt recording state did not change"
        QApplication.processEvents()
        sleep(0.02)


def test_desktop_stops_and_exports_importable_json(desktop_recording, monkeypatch, tmp_path):
    from PySide6.QtWidgets import QFileDialog

    window, browser_closed = desktop_recording
    recorder = window.recorder
    output = tmp_path / "录制草稿.json"
    destinations = iter(("", str(output)))
    monkeypatch.setattr(QFileDialog, "getSaveFileName", lambda *args: (next(destinations), ""))

    recorder.start_button.click()
    wait_for_recorder(recorder.export_button.isEnabled)
    recorder.export_button.click()
    wait_for_recorder(lambda: browser_closed and recorder.export_button.isEnabled())
    assert not output.exists()
    recorder.export_button.click()

    draft = json.loads(output.read_text(encoding="utf-8"))
    assert [step["action"] for step in draft["steps"]] == ["navigate", "fill", "click", "navigate", "click"]
    assert draft["steps"][1] == {"action": "fill", "role": "textbox", "name": "账号", "data_key": "account"}
    assert "synthetic-" not in output.read_text(encoding="utf-8")


@pytest.mark.parametrize("exit_app", [False, True], ids=["discard", "app-quit"])
def test_desktop_discards_and_closes_browser(desktop_recording, recorder_qt, monkeypatch, exit_app):
    from PySide6.QtCore import QTimer
    from PySide6.QtWidgets import QFileDialog

    window, browser_closed = desktop_recording
    recorder = window.recorder
    saves = []
    monkeypatch.setattr(QFileDialog, "getSaveFileName", lambda *args: (saves.append(args) or "", ""))
    recorder.start_button.click()
    wait_for_recorder(recorder.export_button.isEnabled)

    if exit_app:
        QTimer.singleShot(0, recorder_qt.quit)
        recorder_qt.exec()
    else:
        recorder.discard_button.click()
        wait_for_recorder(lambda: not recorder.isVisible())

    assert browser_closed == [True]
    assert not saves


@pytest.mark.parametrize("desktop_recording", [True], indirect=True, ids=["close-tab"])
def test_desktop_keeps_draft_for_export_after_tab_closes(desktop_recording):
    window, browser_closed = desktop_recording
    recorder = window.recorder
    recorder.start_button.click()
    wait_for_recorder(lambda: browser_closed and recorder.export_button.isEnabled())

    assert recorder.export_button.text() == "导出 JSON"
    assert recorder.draft["steps"][-1] == {"action": "click", "role": "button", "name": "完成"}


def test_desktop_stops_while_target_is_loading(desktop_recording, recorder_qt, caplog):
    from PySide6.QtCore import QTimer

    window, _ = desktop_recording
    recorder = window.recorder
    requested = Event()
    release = Event()

    class SlowPage(SimpleHTTPRequestHandler):
        def do_GET(self):
            requested.set()
            release.wait(25)

    with ThreadingHTTPServer(("127.0.0.1", 0), SlowPage) as server:
        thread = Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            recorder.address.setText(f"http://127.0.0.1:{server.server_port}/")
            recorder.start_button.click()
            worker = recorder.worker
            wait_for_recorder(requested.is_set)
            assert not recorder.export_button.isEnabled()

            started = monotonic()
            QTimer.singleShot(0, recorder_qt.quit)
            recorder_qt.exec()
            assert monotonic() - started < 10
            assert not worker.isRunning()
            assert worker.draft is None
            assert "Future exception was never retrieved" not in caplog.text
        finally:
            release.set()
            server.shutdown()
            thread.join()


def test_desktop_rejects_invalid_url_and_allows_retry(desktop_recording):
    window, browser_closed = desktop_recording
    recorder = window.recorder
    recorder.address.setText("file:///index.html")
    recorder.start_button.click()
    wait_for_recorder(recorder.start_button.isEnabled)

    assert "HTTP(S)" in recorder.status.text()
    assert not recorder.export_button.isEnabled()
    assert not browser_closed
