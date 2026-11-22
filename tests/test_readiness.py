import asyncio
from urllib.parse import quote

import pytest
from playwright.async_api import async_playwright

from auto_ai.browser import BrowserSession
from auto_ai.config import config
from auto_ai.readiness import wait_for_ready


@pytest.fixture
async def page():
    async with async_playwright() as playwright:
        browser = await playwright.chromium.launch(headless=True)
        page = await browser.new_page()
        try:
            yield page
        finally:
            await browser.close()


@pytest.mark.asyncio
async def test_blank_page_times_out_as_blank(page):
    await page.set_content("<body></body>")

    result = await wait_for_ready(page, timeout_ms=80, stable_ms=20)

    assert result["status"] == "timeout"
    assert result["reason"] == "blank"
    assert result["controls"] == 0


@pytest.mark.asyncio
async def test_loading_page_times_out_as_loading(page):
    await page.set_content('<body aria-busy="true"><button>登录</button></body>')

    result = await wait_for_ready(page, timeout_ms=80, stable_ms=20)

    assert result == {
        "status": "timeout",
        "reason": "loading",
        "controls": 1,
        "busy": True,
        "duration_ms": result["duration_ms"],
        "stable_ms": result["stable_ms"],
    }
    assert 80 <= result["duration_ms"] < 180


@pytest.mark.asyncio
async def test_page_becomes_ready_after_aria_busy_clears(page):
    await page.set_content(
        '<body aria-busy="true"><button>登录</button>'
        '<script>setTimeout(() => document.body.removeAttribute("aria-busy"), 25)</script>'
        '</body>'
    )

    result = await wait_for_ready(page, timeout_ms=200, stable_ms=30)

    assert result["status"] == "ready"
    assert result["reason"] is None
    assert result["busy"] is False
    assert result["controls"] == 1


@pytest.mark.asyncio
async def test_interactive_controls_must_remain_stable(page):
    await page.set_content(
        "<button id='action'>开始</button>"
        "<script>"
        "setTimeout(() => document.getElementById('action').textContent = '继续', 25)"
        "</script>"
    )

    result = await wait_for_ready(page, timeout_ms=200, stable_ms=50)

    assert result["status"] == "ready"
    assert result["reason"] is None
    assert result["controls"] == 1
    assert result["stable_ms"] >= 50


@pytest.mark.asyncio
async def test_changing_control_state_times_out_as_unstable(page):
    await page.set_content(
        "<button id='action'>开始</button>"
        "<script>"
        "setInterval(() => document.getElementById('action').textContent = "
        "document.getElementById('action').textContent === '开始' ? '继续' : '开始', 37)"
        "</script>"
    )

    result = await wait_for_ready(page, timeout_ms=140, stable_ms=80)

    assert result["status"] == "timeout"
    assert result["reason"] == "unstable"


@pytest.fixture
def session(page, tmp_path, monkeypatch):
    monkeypatch.setattr(config, "NEO4J_ENABLED", False)
    session = BrowserSession(cache_dir=tmp_path)
    session.page = page
    return session


@pytest.mark.asyncio
async def test_navigate_waits_for_single_delayed_control(session):
    html = (
        '<body aria-busy="true"><script>setTimeout(() => {'
        'document.body.innerHTML = "<button>Continue</button>";'
        'document.body.removeAttribute("aria-busy");'
        '}, 850)</script></body>'
    )

    snapshot, is_cached = await asyncio.wait_for(
        session.navigate("data:text/html," + quote(html)), timeout=5
    )

    assert "Continue" in snapshot
    assert is_cached is False


@pytest.mark.asyncio
async def test_navigate_returns_snapshot_for_read_only_page(session):
    url = "data:text/html," + quote("<title>Read only</title><p>Static content</p>")

    snapshot, is_cached = await session.navigate(url)

    assert await session.page.locator("p").inner_text() == "Static content"
    assert isinstance(snapshot, str)
    assert is_cached is False
