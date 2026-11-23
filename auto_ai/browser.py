"""Playwright 浏览器管理 + 原子操作 + API 追踪"""

import asyncio
import os
from pathlib import Path
from contextlib import asynccontextmanager
from contextvars import ContextVar
from typing import Optional

from playwright.async_api import async_playwright, Browser, BrowserContext, Page

from . import snapshot as snap
from .readiness import wait_for_ready

# ---------- API 请求记录 ----------

class APITracker:
    """每次操作前清空，操作后收集触发的 API 请求、响应状态码和响应体"""
    def __init__(self):
        self.requests: list[dict] = []
        self._pending: dict[str, dict] = {}  # request_url -> 等待响应的请求
        self._page: Optional[Page] = None

    def attach(self, page: Page):
        self._page = page
        page.on("request", self._on_request)
        page.on("response", self._on_response)

    def detach(self):
        if self._page:
            self._page.remove_listener("request", self._on_request)
            self._page.remove_listener("response", self._on_response)
            self._page = None

    def _on_request(self, req):
        url = req.url
        # 跳过浏览器资源（字体、图片、CSS、JS、gif tracker）
        skip_exts = (".css", ".js", ".woff2", ".png", ".svg", ".ico", ".ttf", ".gif")
        if any(url.endswith(ext) for ext in skip_exts):
            return
        if "hm.gif" in url or "hm.baidu.com" in url:
            return
        self._pending[url] = {
            "url": url,
            "method": req.method,
            "post_data": req.post_data,
            "status": 0,
            "body": None,
        }

    def _on_response(self, resp):
        url = resp.url
        if url in self._pending:
            entry = self._pending[url]
            entry["status"] = resp.status
            # 抓 JSON API 的响应体（限 100KB，含错误响应）
            content_type = resp.headers.get("content-type", "")
            if "json" in content_type:
                try:
                    body_text = resp.text()
                    if len(body_text) < 100_000:
                        entry["body"] = body_text
                except Exception:
                    pass
            self.requests.append(self._pending.pop(url))

    def start_capture(self):
        self.requests.clear()

    def get_apis(self) -> list[dict]:
        return list(self.requests)

    def extract_filter_options(self) -> list[dict]:
        """从 API 响应中提取筛选选项（combo/select 的动态数据源）"""
        options = []
        for req in self.requests:
            body = req.get("body")
            if not body:
                continue
            try:
                data = json.loads(body)
                # 递归查找包含 id+name / value+label 的结构
                rows = self._find_option_lists(data)
                for row in rows:
                    name = row.get("name") or row.get("label") or row.get("title") or ""
                    value = row.get("id") or row.get("value") or row.get("code") or ""
                    if name and isinstance(name, str) and len(name) < 30:
                        options.append({
                            "api_url": req["url"],
                            "name": name.strip(),
                            "value": str(value),
                        })
            except Exception:
                pass
        return options

    @staticmethod
    def _find_option_lists(data):
        """递归查找疑似选项列表的数组"""
        if isinstance(data, list) and len(data) > 0:
            first = data[0]
            if isinstance(first, dict):
                # 判断是否像选项：有 name/label/title
                if any(k in first for k in ("name", "label", "title")):
                    return data
            # 递归到子字段中的数组
            for item in data:
                yield from APITracker._find_option_lists(item)
        elif isinstance(data, dict):
            for v in data.values():
                yield from APITracker._find_option_lists(v)
        return []


_api_tracker = APITracker()
worker_browser: ContextVar[str] = ContextVar("worker_browser", default="")


class BrowserSession:
    """Playwright 浏览器会话，封装浏览器生命周期和常用操作。"""

    def __init__(self, headless: bool = True, cache_dir: Path = Path("cache")):
        self.headless = headless
        self.cache_dir = cache_dir
        self._playwright = None
        self._browser: Optional[Browser] = None
        self._context: Optional[BrowserContext] = None
        self.page: Optional[Page] = None
        self.last_locate_detail: dict = {"tried": [], "used": None}

    async def start(self):
        """启动浏览器"""
        browser_name = worker_browser.get() or os.environ.get("PLATFORM_BROWSER", "chromium").strip().lower()
        if browser_name not in {"chromium", "firefox", "webkit", "chrome", "msedge"}:
            raise ValueError(f"不支持的 Playwright 浏览器：{browser_name}")
        self._playwright = await async_playwright().start()
        try:
            if browser_name in {"chrome", "msedge"}:
                self._browser = await self._playwright.chromium.launch(headless=self.headless, channel=browser_name)
            else:
                self._browser = await getattr(self._playwright, browser_name).launch(headless=self.headless)
        except Exception:
            await self._playwright.stop()
            raise
        self._context = await self._browser.new_context(
            viewport={"width": 1280, "height": 720}
        )
        self.page = await self._context.new_page()
        # 挂上 API 追踪器
        _api_tracker.attach(self.page)
        # 加载缓存
        snap._load_cache_from_disk(self.cache_dir)

    async def stop(self):
        """关闭浏览器"""
        _api_tracker.detach()
        if self._context:
            await self._context.close()
        if self._browser:
            await self._browser.close()
        if self._playwright:
            await self._playwright.stop()
        # 取消残留的 asyncio 任务，避免 Windows Event loop is closed 警告
        pending = [t for t in asyncio.all_tasks()
                   if t is not asyncio.current_task()]
        for t in pending:
            t.cancel()
        if pending:
            await asyncio.gather(*pending, return_exceptions=True)

    # ---------- 原子操作 ----------

    async def _wait_for_change(self, timeout: int = 10) -> bool:
        """
        等待页面内容发生变化（用于 AJAX 操作后）。
        对比操作前后的元素指纹，直到不同或超时。
        """
        # 1. 先等网络空闲（AJAX 请求完成）
        try:
            await self.page.wait_for_load_state("networkidle", timeout=timeout * 1000)
        except Exception:
            pass

        # 2. 轮询确认内容真的变了（对比指纹）
        before_fp = snap.make_fingerprint(snap.flatten_elements(await snap.extract_page_structure(self.page)))
        deadline = asyncio.get_event_loop().time() + timeout
        while asyncio.get_event_loop().time() < deadline:
            await asyncio.sleep(0.5)
            structure = await snap.extract_page_structure(self.page)
            current_fp = snap.make_fingerprint(snap.flatten_elements(structure))
            if current_fp != before_fp:
                return True
            # fingerprint 相同时更新 before_fp（页面可能持续在变）
            before_fp = current_fp
        return False

    async def navigate(self, url: str):
        """导航到指定 URL，等待动态内容加载完成"""
        old_url = self.page.url
        try:
            await self.page.goto(url, wait_until="domcontentloaded", timeout=30000)
        except Exception:
            pass
        await wait_for_ready(self.page, timeout_ms=15_000)

        # 记录导航边到知识图谱
        new_url = self.page.url
        if old_url and old_url != new_url:
            try:
                from .knowledge_graph import get_kg
                get_kg().record_navigation(old_url, new_url, "navigate", url)
            except Exception:
                pass

        return await snap.get_snapshot(self.page, self.cache_dir)

    async def get_page_state(self, force_refresh: bool = False) -> tuple[str, bool]:
        """获取当前页面状态，force_refresh=True 时跳过缓存"""
        return await snap.get_snapshot(self.page, self.cache_dir, force_refresh=force_refresh)

    # ---------- 自愈定位器 ----------

    async def _locate_element(self, role: str, name: str):
        """
        多策略定位元素，内置自愈降级链。

        降级顺序：
            ① get_by_role(role, name)       -- 精确 role 匹配
            ② get_by_text(name, exact=True)  -- 精确文本匹配
            ③ get_by_text(name)              -- 模糊文本匹配
            ④ 滚动页面 + 重试 ①②③（最多 3 次滚动）
            ⑤ 放弃，返回 None（由上层做 LLM 兜底）

        返回: (locator, strategy_name) 或 (None, None)
        """
        tried = []

        # 策略 ①：role + name
        if role:
            tried.append(f"role({role}, {name})")
            loc = self.page.get_by_role(role, name=name)
            if await loc.count() > 0:
                self.last_locate_detail = {"tried": tried, "used": "role"}
                return loc, "role"

        # 策略 ②：精确文本匹配
        tried.append(f"text_exact({name})")
        loc = self.page.get_by_text(name, exact=True)
        if await loc.count() > 0:
            self.last_locate_detail = {"tried": tried, "used": "text_exact"}
            return loc, "text_exact"

        # 策略 ③：模糊文本匹配
        tried.append(f"text_fuzzy({name})")
        loc = self.page.get_by_text(name)
        if await loc.count() > 0:
            self.last_locate_detail = {"tried": tried, "used": "text_fuzzy"}
            return loc, "text_fuzzy"

        # 策略 ④：滚动页面并重试
        viewport = self.page.viewport_size or {"width": 1280, "height": 720}
        for scroll_i in range(3):
            # 向下滚动一屏
            await self.page.evaluate(f"window.scrollBy(0, {viewport['height'] * 0.7})")
            await asyncio.sleep(0.6)

            if role:
                tried.append(f"scroll{scroll_i+1}_role")
                loc = self.page.get_by_role(role, name=name)
                if await loc.count() > 0:
                    self.last_locate_detail = {"tried": tried, "used": f"scroll{scroll_i+1}_role"}
                    return loc, f"scroll{scroll_i+1}_role"

            tried.append(f"scroll{scroll_i+1}_text_exact")
            loc = self.page.get_by_text(name, exact=True)
            if await loc.count() > 0:
                self.last_locate_detail = {"tried": tried, "used": f"scroll{scroll_i+1}_text_exact"}
                return loc, f"scroll{scroll_i+1}_text_exact"

            tried.append(f"scroll{scroll_i+1}_text_fuzzy")
            loc = self.page.get_by_text(name)
            if await loc.count() > 0:
                self.last_locate_detail = {"tried": tried, "used": f"scroll{scroll_i+1}_text_fuzzy"}
                return loc, f"scroll{scroll_i+1}_text_fuzzy"

        # 全部失败
        self.last_locate_detail = {"tried": tried, "used": None}
        return None, None

    # ---------- 知识图谱记录 ----------

    def _record_kg(self, old_url: str, role: str, name: str):
        """记录操作到 Neo4j 知识图谱（导航边 + API 调用 + 筛选选项）"""
        try:
            from .knowledge_graph import get_kg
            kg = get_kg()
            kg_kg = role if role else "link"
            kg.mark_explored(old_url, kg_kg, name)
            new_url = self.page.url
            if new_url != old_url:
                kg.record_navigation(old_url, new_url, kg_kg, name)
            for api in _api_tracker.get_apis():
                kg.record_api(old_url, kg_kg, name, api["url"], api["method"])
            # 从 API 响应中提取筛选选项并添加为页面元素
            filter_opts = _api_tracker.extract_filter_options()
            for opt in filter_opts:
                kg.upsert_element(new_url, "option", opt["name"], locator=None)
        except Exception:
            pass

    # ---------- 自愈点击 ----------

    async def click_smart(self, role: str, name: str) -> str:
        """智能点击：多策略定位 + 自愈降级，返回操作描述"""
        old_url = self.page.url
        _api_tracker.start_capture()

        locator, strategy = await self._locate_element(role, name)
        if locator is None:
            return f"失败：自愈定位全部失败，未找到 '{name}' (role={role})"

        await locator.first.click()
        await self._wait_for_change()

        self._record_kg(old_url, role, name)
        return f"成功：通过 [{strategy}] 点击了 '{name}'"

    # ---------- 自愈填入 ----------

    async def fill_smart(self, role: str, name: str, value: str) -> str:
        """智能填入：多策略定位 + 自愈降级，返回操作描述"""
        _api_tracker.start_capture()

        locator, strategy = await self._locate_element(role, name)
        if locator is None:
            return f"失败：自愈定位全部失败，未找到输入框 '{name}' (role={role})"

        await locator.first.fill(value)
        return f"成功：通过 [{strategy}] 在 '{name}' 中填入了 '{value}'"

    async def screenshot(self, name: str) -> str:
        """截图"""
        return await snap.take_screenshot(self.page, name)

    async def wait_for(self, text: str, timeout: int = 5000) -> bool:
        """等待文本出现"""
        try:
            await self.page.wait_for_selector(f"text={text}", timeout=timeout)
            return True
        except Exception:
            return False

    # ---------- 断言辅助方法 ----------

    def get_current_url(self) -> str:
        """获取当前页面完整 URL"""
        return self.page.url

    async def check_element_attribute(self, name: str, attribute: str) -> tuple[bool, str]:
        """检查页面中某个元素的属性状态。
        返回 (found, message)
        attribute 支持: disabled, enabled, checked, visible, hidden
        """
        js = """([name, attr]) => {
            const all = document.querySelectorAll(
                'a, button, input, select, textarea, [role], h1, h2, h3, h4, h5, h6'
            );
            for (const el of all) {
                let elName = el.getAttribute('aria-label') || el.textContent || el.getAttribute('placeholder') || el.getAttribute('title') || '';
                elName = elName.replace(/\\s+/g, ' ').trim();
                if (elName === name) {
                    if (attr === 'disabled') return [true, el.disabled];
                    if (attr === 'enabled') return [true, !el.disabled];
                    if (attr === 'checked') return [true, el.checked === true];
                    if (attr === 'visible') return [true, el.offsetParent !== null];
                    if (attr === 'hidden') return [true, el.offsetParent === null];
                    return [true, null];  // unknown attr
                }
            }
            return [false, null];
        }"""
        result = await self.page.evaluate(js, [name, attribute])
        found = result[0]
        value = result[1]
        return (found, f"'{name}' {attribute}={value}" if found else f"未找到元素 '{name}'")

    async def count_table_rows(self, table_name: str) -> int:
        """根据表格名称（相邻的 caption/heading 文本）查找表格并统计行数。
        匹配逻辑：先找包含 table_name 文本的元素，然后找其后最近的 table，
        或找 caption 包含 table_name 的 table。
        返回行数（tbody tr 数量），未找到返回 -1。
        """
        js = """(name) => {
            name = name.toLowerCase();
            // 策略1: 找 caption 包含 name 的 table
            const captions = document.querySelectorAll('table caption');
            for (const cap of captions) {
                if (cap.textContent.toLowerCase().includes(name)) {
                    const tbody = cap.closest('table').querySelector('tbody');
                    return (tbody || cap.closest('table')).querySelectorAll('tr').length;
                }
            }
            // 策略2: 找 heading 包含 name，再找其后最近的 table
            const headings = document.querySelectorAll('h1,h2,h3,h4,h5,h6,*[role="heading"]');
            for (const h of headings) {
                if (h.textContent.toLowerCase().includes(name)) {
                    // 找该 heading 后第一个 table（同层级或子级）
                    let next = h.nextElementSibling;
                    let depth = 10;
                    while (next && depth > 0) {
                        const table = next.tagName === 'TABLE' ? next : next.querySelector('table');
                        if (table) {
                            const tbody = table.querySelector('tbody');
                            return (tbody || table).querySelectorAll('tr').length;
                        }
                        next = next.nextElementSibling;
                        depth--;
                    }
                    // 也检查父级中的 table
                    const parentTable = h.closest('section,div,main')?.querySelector('table');
                    if (parentTable) {
                        const tbody = parentTable.querySelector('tbody');
                        return (tbody || parentTable).querySelectorAll('tr').length;
                    }
                }
            }
            // 策略3: 找 table 中第一个 th/td 包含 name 的
            const tables = document.querySelectorAll('table');
            for (const table of tables) {
                const firstCell = table.querySelector('th, td');
                if (firstCell && firstCell.textContent.toLowerCase().includes(name)) {
                    const tbody = table.querySelector('tbody');
                    return (tbody || table).querySelectorAll('tr').length;
                }
            }
            return -1;
        }"""
        return await self.page.evaluate(js, table_name)


# ---------- 便捷上下文管理器 ----------

@asynccontextmanager
async def browser_session(headless: bool = True, cache_dir: Path = Path("cache")):
    session = BrowserSession(headless=headless, cache_dir=cache_dir)
    await session.start()
    try:
        yield session
    finally:
        await session.stop()
