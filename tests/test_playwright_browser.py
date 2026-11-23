from pathlib import Path

import pytest
from playwright.async_api import async_playwright

from agents.worker_credentials import execute_configured_ui
from auto_ai.browser import browser_session


@pytest.mark.asyncio
async def test_explicit_playwright_browser_overrides_worker_default(tmp_path, monkeypatch):
    async with async_playwright() as playwright:
        if not Path(playwright.firefox.executable_path).is_file():
            pytest.skip("Playwright Firefox is not installed")

    monkeypatch.setenv("PLATFORM_BROWSER", "chromium")

    async def run_ui(task, settings):
        async with browser_session(cache_dir=tmp_path) as session:
            assert session.page.context.browser.browser_type.name == "firefox"
            await session.page.goto("data:text/html,<p>Firefox smoke</p>")
            assert await session.page.get_by_text("Firefox smoke", exact=True).is_visible()
        return {"status": "passed"}

    assert await execute_configured_ui(run_ui, {}, {"playwright_browser": "firefox"}) == {"status": "passed"}


@pytest.mark.asyncio
async def test_unspecified_browser_preserves_worker_default(tmp_path, monkeypatch):
    monkeypatch.setenv("PLATFORM_BROWSER", "chromium")
    async with browser_session(cache_dir=tmp_path) as session:
        assert session.page.context.browser.browser_type.name == "chromium"


@pytest.mark.asyncio
async def test_worker_reports_unavailable_optional_providers(tmp_path, monkeypatch):
    monkeypatch.setenv("PLATFORM_BROWSER", "invalid-default")

    async def run_ui(task, settings):
        async with browser_session(cache_dir=tmp_path) as session:
            await session.page.goto("data:text/html,<p>Worker smoke</p>")
            assert await session.page.get_by_text("Worker smoke", exact=True).is_visible()
            assert session.page.context.browser.browser_type.name == "chromium"
        return {"status": "passed", "fallback_reason": "Existing reason", "report": "Completed"}

    result = await execute_configured_ui(run_ui, {}, {
        "playwright_browser": "chromium", "ego_enabled": True, "jev_enabled": True,
    })

    assert result["status"] == "passed"
    for reason in ("Ego", "Playwright", "JEV", "Existing reason"):
        assert reason in result["fallback_reason"]
    assert result["report"].startswith(result["fallback_reason"])
    with pytest.raises(ValueError, match="invalid-default"):
        async with browser_session(cache_dir=tmp_path):
            pytest.fail("worker browser preference leaked after the run")
