"""Record a draft UI scenario in a fresh local Chromium session."""

import asyncio
from collections.abc import Awaitable, Callable
from queue import Empty, SimpleQueue
import sys
from threading import Thread
from urllib.parse import urlsplit

from playwright.async_api import Error, Page, async_playwright

from .recorded_steps import convert_recorded_steps


_CAPTURE = r"""
(() => {
    if (window !== window.top) return;

    const clickRoles = new Set(['button', 'link', 'checkbox', 'radio', 'tab', 'menuitem', 'switch']);
    const inputRoles = {
        text: 'textbox', email: 'textbox', tel: 'textbox', url: 'textbox',
        search: 'searchbox', number: 'spinbutton', checkbox: 'checkbox', radio: 'radio',
    };
    let lastInput;

    function roleOf(element) {
        const explicit = element.getAttribute('role')?.trim().split(/\s+/)[0];
        if (explicit) return explicit;
        if (element.matches('button')) return 'button';
        if (element.matches('a[href]')) return 'link';
        if (element.matches('textarea')) return 'textbox';
        if (element.matches('input')) return inputRoles[element.type];
    }

    function labelText(node) {
        if (node.nodeType === Node.TEXT_NODE) return node.textContent;
        if (!(node instanceof Element) || node.isContentEditable || node.matches(
            'input, textarea, select, script, style, [hidden], [aria-hidden="true"]'
        )) return '';
        if (node.matches('img')) return node.getAttribute('alt') || '';
        return Array.from(node.childNodes, labelText).join('');
    }

    function accessibleName(element, allowContent) {
        const ids = element.getAttribute('aria-labelledby')?.trim().split(/\s+/) || [];
        const root = element.getRootNode();
        const referenced = ids.map(id => root.getElementById(id)).filter(Boolean)
            .map(label => label.getAttribute('aria-label') || labelText(label)).join(' ').trim();
        const labels = Array.from(element.labels || [], labelText).join(' ').trim();
        const name = referenced || element.getAttribute('aria-label')?.trim() || labels ||
            (allowContent ? labelText(element) : '');
        return name.replace(/\s+/g, ' ').trim();
    }

    window.addEventListener('click', event => {
        if (!event.isTrusted || event.button !== 0) return;
        const element = event.composedPath().find(node => node instanceof Element && clickRoles.has(roleOf(node)));
        if (!element) return;
        const name = accessibleName(element, true);
        if (!name) return;
        lastInput = undefined;
        void window.__autoTestRecord('click', roleOf(element), name).catch(() => {});
    }, true);

    window.addEventListener('input', event => {
        const element = event.target;
        if (!event.isTrusted || !element.matches('input, textarea') || lastInput === element) return;
        const role = roleOf(element);
        if (!['textbox', 'searchbox', 'spinbutton'].includes(role)) return;
        const name = accessibleName(element, false);
        if (!name) return;
        lastInput = element;
        const dataKey = element.getAttribute('data-key')?.trim() || element.name.trim() || element.id.trim() || name;
        void window.__autoTestRecord('fill', role, name, dataKey).catch(() => {});
    }, true);
})();
"""


async def record_ui_case(
    url: str, wait_for_stop: Callable[[Page], Awaitable[bool]], *, headless: bool = False,
) -> dict | None:
    parts = urlsplit(url)
    if parts.scheme not in ("http", "https") or not parts.hostname or parts.username is not None:
        raise ValueError("录制需要不含账号密码的 HTTP(S) URL")

    async with async_playwright() as playwright:
        browser = await playwright.chromium.launch(headless=headless)
        try:
            context = await browser.new_context()
            page = await context.new_page()
            events = await _capture_events(page)
            await page.goto(url, wait_until="domcontentloaded")
            return convert_recorded_steps(events) if await wait_for_stop(page) else None
        finally:
            await browser.close()


async def _capture_events(page: Page) -> list[dict[str, str]]:
    events = []

    def on_navigation(frame):
        if frame != page.main_frame:
            return
        parts = urlsplit(frame.url)
        if parts.scheme in ("http", "https"):
            host = parts.netloc.rsplit("@", 1)[-1]
            events.append({"action": "navigate", "url": f"{parts.scheme}://{host}{parts.path or '/'}"})

    def on_action(source, action, role, name, data_key=None):
        if source["frame"] != page.main_frame:
            return
        escaped_name = name.replace('"', r'\"')
        event = {"action": action, "selector": f'role={role}[name="{escaped_name}"]'}
        if action == "fill":
            event["data_key"] = data_key
        events.append(event)

    page.on("framenavigated", on_navigation)
    await page.expose_binding("__autoTestRecord", on_action)
    await page.add_init_script(_CAPTURE)
    return events


def record_from_terminal(url: str) -> dict | None:
    try:
        return asyncio.run(record_ui_case(url, _wait_for_terminal))
    except KeyboardInterrupt:
        raise SystemExit(130) from None
    except Error:
        raise ValueError("Chromium 录制失败，请检查目标页面和 Playwright Chromium 安装") from None


async def _wait_for_terminal(page: Page) -> bool:
    decision = SimpleQueue()

    def read_command():
        for line in sys.stdin:
            command = line.strip().casefold()
            if command in ("stop", "quit"):
                decision.put(command == "stop")
                return
            print("请输入 stop 或 quit。", file=sys.stderr, flush=True)
        decision.put(False)

    print("录制中。输入 stop 导出草稿，输入 quit 放弃，关闭录制标签页也会导出。", file=sys.stderr, flush=True)
    Thread(target=read_command, daemon=True).start()
    while not page.is_closed():
        try:
            return decision.get_nowait()
        except Empty:
            await asyncio.sleep(0.1)
    return True
