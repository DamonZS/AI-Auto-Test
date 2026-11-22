"""页面快照 + 指纹 + 缓存模块 —— 含区域（Section）层级"""

import hashlib
import json
from pathlib import Path
from urllib.parse import urlparse
from typing import Optional

from playwright.async_api import Page

# ---------- 可交互的 role ----------
INTERACTIVE_ROLES = {
    "textbox", "button", "link", "checkbox", "radio",
    "combobox", "heading", "searchbox", "option",
    "switch", "tab", "slider", "spinbutton",
}

# ---------- 缓存存储 ----------
_cache: dict[str, dict] = {}


def _save_cache_to_disk(cache_dir: Path):
    cache_dir.mkdir(parents=True, exist_ok=True)
    cache_file = cache_dir / "page_templates.json"
    cache_file.write_text(json.dumps(_cache, ensure_ascii=False, indent=2), encoding="utf-8")


def _load_cache_from_disk(cache_dir: Path):
    global _cache
    cache_file = cache_dir / "page_templates.json"
    if cache_file.exists():
        _cache = json.loads(cache_file.read_text(encoding="utf-8"))


def normalize_url(full_url: str) -> str:
    """去掉参数、域名，只保留路径骨架"""
    parsed = urlparse(full_url)
    return parsed.path


# ================================================================
# DOM 提取 —— 替代 page.accessibility.snapshot()
# 返回分段结构: {ungrouped: [...], sections: [{section, section_tag, elements: [...]}]}
# ================================================================

_EVAL_SCRIPT = r"""() => {
    const INTERACTIVE = new Set(['button','link','textbox','checkbox','radio',
        'combobox','heading','searchbox','option','switch','tab']);
    const TAG_ROLE = {
        'a':'link','button':'button','select':'combobox','textarea':'textbox',
        'h1':'heading','h2':'heading','h3':'heading','h4':'heading','h5':'heading','h6':'heading'
    };
    const SECTION_TAGS = new Set(['nav','form','table','header','footer','aside','main']);
    const SECTION_ROLES = new Set(['search','listbox','tablist','menu','menubar','toolbar']);
    const seen = new Set();
    // elements 持有 {el, data}，后续按区域分组
    const elements = [];

    document.querySelectorAll(
        'a, button, input, select, textarea, [role], h1, h2, h3, h4, h5, h6, [aria-label]'
    ).forEach(function(el) {
        if (el.getAttribute('aria-hidden') === 'true') return;

        var role = el.getAttribute('role') || el.tagName.toLowerCase();
        role = TAG_ROLE[role] || role;
        if (role === 'input') {
            var t = (el.getAttribute('type') || 'text').toLowerCase();
            if (t === 'submit' || t === 'button' || t === 'reset') role = 'button';
            else if (t === 'checkbox') role = 'checkbox';
            else if (t === 'radio') role = 'radio';
            else role = 'textbox';
        }
        if (!INTERACTIVE.has(role)) return;

        // 提取名称 (同之前)
        var name = '';
        if (el.tagName === 'INPUT' && el.getAttribute('type') !== 'submit' && el.getAttribute('type') !== 'button') {
            if (el.id) {
                var label = document.querySelector('label[for="' + el.id + '"]');
                if (label) name = (label.textContent || '').trim();
            }
            name = name || el.getAttribute('placeholder') || el.getAttribute('aria-label') || el.getAttribute('name') || '';
        } else {
            name = el.getAttribute('aria-label') || el.textContent || el.getAttribute('placeholder') || el.getAttribute('title') || el.getAttribute('alt') || '';
        }
        name = name.replace(/\s+/g, ' ').trim().slice(0, 60);
        if (!name) return;
        if (/^[\d\s\.\,\+\-\*\/\#]+$/.test(name)) return;
        if (name.length < 2 && !/[\u4e00-\u9fff]/.test(name)) return;

        var key = role + '|' + name;
        if (seen.has(key)) return;
        seen.add(key);

        elements.push({
            el: el,
            data: {
                role: role,
                name: name,
                id: el.id || '',
                classes: (el.className && typeof el.className === 'string') ? el.className.split(/\s+/).slice(0,3).join('.') : '',
                tag: el.tagName.toLowerCase()
            }
        });
    });

    // ---- 查找元素所属区域 ----
    function findSection(el) {
        var parent = el.parentElement;
        var depth = 7;
        while (parent && depth > 0) {
            var tag = parent.tagName.toLowerCase();
            var role = parent.getAttribute('role');
            if (tag === 'nav')   return {type: 'nav', name: '\u5bfc\u822a\u680f'};
            if (tag === 'header') return {type: 'header', name: '\u9875\u5934'};
            if (tag === 'footer') return {type: 'footer', name: '\u9875\u5c3e'};
            if (tag === 'aside')  return {type: 'aside', name: '\u4fa7\u8fb9\u680f'};
            if (tag === 'main')   return {type: 'main', name: '\u4e3b\u5185\u5bb9\u533a'};
            if (tag === 'form') {
                var formName = (role === 'search' || (parent.id||'').toLowerCase().indexOf('search') >= 0) ? '\u641c\u7d22\u680f' : '\u8868\u5355';
                return {type: 'form', name: formName};
            }
            if (tag === 'table') return {type: 'table', name: '\u8868\u683c'};
            if (tag === 'section') {
                // 尝试从 aria-label / heading 子元素 取区域名
                var h = parent.querySelector('h1,h2,h3,h4,h5,h6');
                var sec = parent.getAttribute('aria-label') || (h ? h.textContent.trim() : '');
                return {type: 'section', name: sec || '\u533a\u57df'};
            }
            if (SECTION_ROLES.has(role)) {
                var m = {'search': '\u641c\u7d22\u680f','listbox': '\u5217\u8868','tablist': '\u6807\u7b7e\u9875','menu': '\u83dc\u5355','menubar': '\u83dc\u5355\u680f','toolbar': '\u5de5\u5177\u680f'};
                return {type: role, name: m[role] || role};
            }
            if (parent === document.body) break;
            parent = parent.parentElement;
            depth--;
        }
        return null;
    }

    // ---- 分组 ----
    var sectionsMap = {};
    var ungrouped = [];
    elements.forEach(function(item) {
        var sec = findSection(item.el);
        if (sec) {
            var k = sec.type + '|' + sec.name;
            if (!sectionsMap[k]) {
                sectionsMap[k] = {section: sec.name, section_tag: sec.type, elements: []};
            }
            sectionsMap[k].elements.push(item.data);
        } else {
            ungrouped.push(item.data);
        }
    });

    return {ungrouped: ungrouped, sections: Object.values(sectionsMap)};
}"""


async def extract_page_structure(page: Page) -> dict:
    """用 JS 从 DOM 提取页面结构（含区域分组）。
    返回: {ungrouped: [{role, name, ...}], sections: [{section, section_tag, elements: [...]}]}
    """
    return await page.evaluate(_EVAL_SCRIPT)


def flatten_elements(structure: dict) -> list[dict]:
    """将分段结构拍平为单一元素列表（用于指纹、定位器、缓存）"""
    flat = list(structure.get("ungrouped", []))
    for sec in structure.get("sections", []):
        flat.extend(sec.get("elements", []))
    return flat


async def enrich_locators(elements: list[dict], page: Page):
    """为元素列表补上 Playwright 定位器字符串"""
    for el in elements:
        role = el["role"]
        name = el["name"]
        try:
            loc = page.get_by_role(role, name=name)
            if await loc.count() > 0:
                el["_locator"] = f'get_by_role("{role}", name="{name}")'
                el["_confidence"] = "high"
                continue
        except Exception:
            pass
        if el.get("id"):
            el["_locator"] = f'page.locator("#{el["id"]}")'
            el["_confidence"] = "medium"
        elif el.get("classes"):
            cls_sel = "." + ".".join(el["classes"].split("."))
            el["_locator"] = f'page.locator("{cls_sel}")'
            el["_confidence"] = "low"
        else:
            el["_locator"] = f'get_by_text("{name}")'
            el["_confidence"] = "low"


# ================================================================
# 指纹 + 快照
# ================================================================

def make_fingerprint(elements: list[dict]) -> str:
    """根据元素 role+name 计算指纹"""
    items = sorted(f"{e['role']}|{e['name']}" for e in elements)
    return hashlib.md5(",".join(items).encode()).hexdigest()[:8]


def slim_snapshot_with_sections(structure: dict) -> str:
    """将分段结构格式化为带区域标题的精简文本"""
    lines = []

    for sec in structure.get("sections", []):
        sec_name = sec.get("section", "")
        sec_tag = sec.get("section_tag", "")
        lines.append(f"\n--- {sec_name} ({sec_tag}) ---")
        for el in sec.get("elements", []):
            loc = el.get("_locator", "")
            lines.append(f"  [{el['role']}] {el['name']}  -> {loc}" if loc else f"  [{el['role']}] {el['name']}")

    if structure.get("ungrouped"):
        lines.append("\n--- 其他 ---")
        for el in structure["ungrouped"]:
            loc = el.get("_locator", "")
            lines.append(f"  [{el['role']}] {el['name']}  -> {loc}" if loc else f"  [{el['role']}] {el['name']}")

    return "\n".join(lines).strip()


# ================================================================
# 主入口
# ================================================================

async def get_snapshot(page: Page, cache_dir: Path = Path("cache"),
                      force_refresh: bool = False) -> tuple[str, bool]:
    """
    获取当前页面快照，优先走缓存。

    force_refresh=True: 跳过缓存，强制重新提取（用于页面通过 AJAX 更新内容后）

    返回: (slim_snapshot_str, is_cached)
    """
    url = page.url
    key = normalize_url(url)

    # 1. 从 DOM 提取页面结构（含区域分组）
    structure = await extract_page_structure(page)
    elements = flatten_elements(structure)
    if not elements:
        return "(页面无可用交互元素)", False

    # 2. 计算指纹
    fp = make_fingerprint(elements)

    # 3. force_refresh 跳过缓存
    if not force_refresh:
        cached = _cache.get(key)
        if cached and cached.get("fingerprint") == fp:
            return cached["slim_snapshot"], True

    # 4. 补定位器 + 生成快照 + 更新缓存
    #    排平后补定位器，但同时更新 structure 中元素的引用
    await enrich_locators(elements, page)
    slim = slim_snapshot_with_sections(structure)

    _cache[key] = {"fingerprint": fp, "slim_snapshot": slim}
    _save_cache_to_disk(cache_dir)

    # 5. 同步到 Neo4j 知识图谱（页面 + 区域 + 元素）
    try:
        from .knowledge_graph import get_kg
        kg = get_kg()
        kg.upsert_page(url, title="", element_count=len(elements))
        # 写入区域及元素
        for sec in structure.get("sections", []):
            sec_name = sec.get("section", "")
            sec_tag = sec.get("section_tag", "")
            kg.upsert_section(url, sec_name, sec_tag)
            for el in sec.get("elements", []):
                kg.upsert_element(url, el["role"], el["name"],
                                   el.get("_locator", ""), el.get("tag", ""),
                                   section_name=sec_name)
        # 未归组元素
        for el in structure.get("ungrouped", []):
            kg.upsert_element(url, el["role"], el["name"],
                               el.get("_locator", ""), el.get("tag", ""))
        # 记录所有区域的元素数，更新覆盖率
        kg.set_page_section_counts(url, structure.get("sections", []))
    except Exception:
        pass  # Neo4j 不可用时静默跳过

    return slim, False


async def take_screenshot(page: Page, name: str, screenshots_dir: Path = Path("screenshots")):
    """保存截图"""
    screenshots_dir.mkdir(parents=True, exist_ok=True)
    path = screenshots_dir / f"{name}.png"
    await page.screenshot(path=str(path), full_page=True)
    return str(path)
