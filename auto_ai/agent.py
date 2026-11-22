"""LangGraph Agent：规划 → 执行 → 分析 → 报告"""

import json
import re
import logging
from typing import TypedDict, List, Optional, Literal
from pathlib import Path

from langgraph.graph import StateGraph, END
from langchain_openai import ChatOpenAI
from langchain_core.messages import HumanMessage, SystemMessage

from .config import config
from .browser import BrowserSession

logger = logging.getLogger(__name__)

# ============================================================
# LLM 输出校验工具
# ============================================================

VALID_ACTIONS = {"navigate", "click", "fill", "wait", "assert"}
VALID_ROLES = {"textbox", "button", "link", "checkbox", "radio",
               "combobox", "listbox", "menuitem", "switch", "tab",
               "heading", "img", "slider", "spinbutton", "searchbox"}

REQUIRED_FIELDS_BY_ACTION = {
    "navigate": ["action"],
    "click":     ["action", "target"],
    "fill":      ["action", "target", "value"],
    "wait":      ["action", "target"],
    "assert":    ["action", "target"],
}


def _strip_markdown_wrap(text: str) -> str:
    """去掉 LLM 输出的 markdown 代码块包裹和前后空白"""
    text = text.strip()
    # 去掉 ```json ... ``` 或 ``` ... ```
    if text.startswith("```"):
        lines = text.split("\n")
        if len(lines) > 1:
            text = "\n".join(lines[1:])
        if text.endswith("```"):
            text = text[:-3]
    return text.strip()


def _extract_json(text: str) -> str:
    """从文本中提取 JSON（处理 LLM 在前面加了解释文字的情况）"""
    text = _strip_markdown_wrap(text)
    # 尝试直接解析
    try:
        json.loads(text)
        return text
    except json.JSONDecodeError:
        pass
    # 尝试用正则提取 JSON 数组或对象
    m = re.search(r'(\[.*\]|\{.*\})', text, re.DOTALL)
    if m:
        return m.group(0)
    return text


def parse_json_safely(text: str, label: str = "LLM 输出") -> dict | list:
    """安全解析 LLM 输出的 JSON，失败时抛清晰的异常"""
    try:
        extracted = _extract_json(text)
        return json.loads(extracted)
    except json.JSONDecodeError as e:
        raise ValueError(
            f"[{label}] JSON 解析失败: {e}\n原始输出前 500 字符:\n{text[:500]}"
        )


def validate_steps(steps: list) -> list[dict]:
    """校验步骤列表，过滤非法步骤，缺失字段补默认值"""
    if not isinstance(steps, list) or len(steps) == 0:
        raise ValueError("步骤列表为空或不是数组")

    validated: list[dict] = []
    for i, step in enumerate(steps, 1):
        if not isinstance(step, dict):
            logger.warning(f"跳过步骤 {i}：非对象类型 {type(step)}")
            continue

        action = step.get("action", "")
        if action not in VALID_ACTIONS:
            logger.warning(f"跳过步骤 {i}：无效 action '{action}'")
            continue

        # 检查必填字段
        required = REQUIRED_FIELDS_BY_ACTION.get(action, [])
        missing = [f for f in required if not step.get(f)]
        if missing:
            logger.warning(f"步骤 {i} ([{action}]) 缺少字段: {missing}")

        # role 校验（非强制）
        role = step.get("role", "")
        if role and role not in VALID_ROLES:
            logger.warning(f"步骤 {i}：未知 role '{role}'，保留但不保证可用")

        validated.append({
            "action": action,
            "target": step.get("target", ""),
            "role": role,
            "value": step.get("value", ""),
            "description": step.get("description", step.get("target", "")),
        })

    if len(validated) == 0:
        raise ValueError("校验后无可用的步骤")

    return validated


def validate_assert_result(parsed: dict) -> tuple[bool, str]:
    """校验 LLM 断言返回"""
    if not isinstance(parsed, dict):
        raise ValueError(f"断言结果不是对象: {type(parsed)}")

    passed = parsed.get("passed")
    if not isinstance(passed, bool):
        raise ValueError(f"断言结果缺少有效的 'passed' 字段: {parsed}")

    reason = str(parsed.get("reason", ""))
    return passed, reason

# ============================================================
# State
# ============================================================

class TestState(TypedDict):
    target_url: str
    test_scenario: str
    steps: List[dict]               # 规划出的测试步骤
    current_step: int
    page_snapshot: str              # 当前页面的精简快照
    is_cached: bool                 # 当前快照是否命中缓存
    history: List[dict]             # 已执行步骤的结果
    report: str                     # 最终报告
    error: Optional[str]            # 错误信息
    token_usage: dict               # token 消耗统计


def _record_tokens(state: TestState, response, label: str):
    """从 AIMessage 提取 token 用量，累积到 state"""
    meta = getattr(response, "response_metadata", {}) or {}
    usage = meta.get("token_usage", {})
    if not usage:
        return
    # 确保 state 中有计数结构
    if "total" not in state["token_usage"]:
        state["token_usage"] = {"total": 0, "prompt": 0, "completion": 0, "details": []}
    state["token_usage"]["prompt"] += usage.get("prompt_tokens", 0)
    state["token_usage"]["completion"] += usage.get("completion_tokens", 0)
    state["token_usage"]["total"] += usage.get("total_tokens", 0)
    state["token_usage"]["details"].append({
        "label": label,
        "prompt": usage.get("prompt_tokens", 0),
        "completion": usage.get("completion_tokens", 0),
    })

# ============================================================
# LLM 实例
# ============================================================

planner_llm = ChatOpenAI(
    model=config.PLANNER_MODEL,
    temperature=config.TEMPERATURE,
    api_key=config.OPENAI_API_KEY,
    base_url=config.OPENAI_BASE_URL,
)

executor_llm = ChatOpenAI(
    model=config.EXECUTOR_MODEL,
    temperature=config.TEMPERATURE,
    api_key=config.OPENAI_API_KEY,
    base_url=config.OPENAI_BASE_URL,
)

# ============================================================
# Planner 节点的 system prompt
# ============================================================

PLANNER_PROMPT = """你是一个自动化测试规划器。用户会用自然语言描述一个测试场景，你需要把它拆解为精确的、可执行的步骤。

输出格式：纯 JSON 数组，每个元素是一个步骤对象：

[
  {
    "action": "navigate | click | fill | wait | assert",
    "target": "元素描述（用于 assert 时是预期文本）",
    "role": "aria role（textbox/button/link/checkbox/combobox）",
    "value": "仅 fill 步骤需要，填入的值",
    "description": "这一步做什么"
  }
]

规则：
1. 第一步永远是 navigate
2. 每个 click 步骤不能跳过中间的跳转页（如点击"登录"后可能跳转到新页面）
3. 每个 click/fill 步骤必须给出 role 和 target（元素上的文字内容）
4. 每个关键操作后加 assert 校验结果
5. 步骤尽量原子化，一个步骤只做一件事
6. 只输出 JSON 数组，不要其他内容"""


async def planner(state: TestState, browser: BrowserSession) -> TestState:
    """将测试场景拆解为结构化步骤，含 JSON 解析校验"""
    print("\n[Planner] 正在拆解测试场景...")

    # 注入知识图谱信息
    graph_context = ""
    try:
        from .knowledge_graph import get_kg
        graph_context = get_kg().get_graph_summary()
    except Exception:
        pass

    prompt = PLANNER_PROMPT
    if graph_context:
        prompt += f"\n\n{graph_context}"

    response = await planner_llm.ainvoke([
        SystemMessage(content=prompt),
        HumanMessage(content=f"测试场景：{state['test_scenario']}\n起始 URL：{state['target_url']}"),
    ])

    _record_tokens(state, response, "Planner 规划")

    raw_text = response.content

    # 1. 安全解析 JSON
    raw_steps = parse_json_safely(raw_text, label="Planner 输出")

    # 2. 校验步骤结构
    steps = validate_steps(raw_steps)

    # 3. 确保第一步是 navigate
    if steps[0]["action"] != "navigate":
        logger.warning("Planner 的第一步不是 navigate，自动补上")
        steps.insert(0, {
            "action": "navigate",
            "target": state["target_url"],
            "role": "",
            "value": "",
            "description": f"导航到 {state['target_url']}",
        })

    state["steps"] = steps
    state["current_step"] = 0
    state["history"] = []
    state["error"] = None

    print(f"[Planner] 生成 {len(steps)} 个步骤：")
    for i, s in enumerate(steps):
        print(f"  {i+1}. [{s['action']}] {s.get('description', s.get('target', ''))}")

    return state


# ============================================================
# Executor 节点
# ============================================================

async def execute_step(state: TestState, browser: BrowserSession) -> TestState:
    """执行当前步骤"""
    steps = state["steps"]
    idx = state["current_step"]

    if idx >= len(steps):
        return state

    step = steps[idx]
    action = step["action"]
    target = step.get("target", "")
    role = step.get("role", "")
    value = step.get("value", "")

    print(f"\n[Executor] 步骤 {idx+1}/{len(steps)}: [{action}] {step.get('description', target)}")

    result = {"step": idx + 1, "action": action, "target": target, "status": "ok", "message": ""}

    # 辅助：更新状态（如果消息包含"失败"）
    def _check_fail(msg: str):
        if msg.startswith("失败"):
            result["status"] = "fail"

    try:
        # ---- navigate ----
        if action == "navigate":
            url = state["target_url"] if not target else target
            snapshot_str, is_cached = await browser.navigate(url)
            state["page_snapshot"] = snapshot_str
            state["is_cached"] = is_cached
            result["message"] = f"导航到 {url}"
            cache_tag = " [缓存命中]" if is_cached else ""
            print(f"  -> 页面快照{cache_tag}：\n{snapshot_str[:300]}...")

        # ---- click ----
        elif action == "click":
            msg = await browser.click_smart(role, target)
            result["message"] = msg
            _check_fail(msg)

            # 自愈：定位失败 → LLM 重定位 → 重试一次
            if msg.startswith("失败"):
                relocated = await _llm_relocate(state, step)
                if relocated:
                    msg2 = await browser.click_smart(relocated["role"], relocated["name"])
                    if msg2.startswith("成功"):
                        result["message"] = msg2
                        result["status"] = "ok"
                        result["_self_healed"] = True
                        msg = msg2

            # 点击后刷新页面状态（强制刷新：页面可能通过 AJAX 变了内容但 URL 没变）
            snapshot_str, is_cached = await browser.get_page_state(force_refresh=True)
            state["page_snapshot"] = snapshot_str
            state["is_cached"] = is_cached
            cache_tag = " [缓存命中]" if is_cached else ""
            print(f"  -> {msg}{cache_tag}")

        # ---- fill ----
        elif action == "fill":
            msg = await browser.fill_smart(role, target, value)
            result["message"] = msg
            _check_fail(msg)

            # 自愈：定位失败 → LLM 重定位 → 重试一次
            if msg.startswith("失败"):
                relocated = await _llm_relocate(state, step)
                if relocated:
                    msg2 = await browser.fill_smart(relocated["role"], relocated["name"], value)
                    if msg2.startswith("成功"):
                        result["message"] = msg2
                        result["status"] = "ok"
                        result["_self_healed"] = True
                        msg = msg2

            print(f"  -> {msg}")

            # 填入后也刷新（强制刷新）
            snapshot_str, is_cached = await browser.get_page_state(force_refresh=True)
            state["page_snapshot"] = snapshot_str
            state["is_cached"] = is_cached

        # ---- wait ----
        elif action == "wait":
            if not target:
                # 目标为空 → 跳过
                result["message"] = "跳过：wait 步骤未指定目标文本"
                print(f"  -> {result['message']}")
            else:
                ok = await browser.wait_for(target)
                result["status"] = "ok" if ok else "fail"
                result["message"] = f"等待 '{target}' {'出现' if ok else '超时'}"

        # ---- assert ----
        elif action == "assert":
            # 先走代码匹配（0 token），匹配不到再降级到 LLM
            result = await _smart_assert(state, step, result)

    except Exception as e:
        # 崩溃检测：浏览器/页面不可恢复 → 立即终止
        exc_name = type(e).__name__
        crash_keywords = ("TargetClosed", "TargetCrashed", "ConnectionClosed", "BrowserClosed")
        if any(k in exc_name for k in crash_keywords) or "page.goto" in str(e) and "Timeout" in exc_name:
            result["status"] = "crash"
            result["message"] = f"浏览器崩溃: {exc_name}: {e}"
            state["error"] = "crash"
        else:
            result["status"] = "error"
            result["message"] = str(e)
        print(f"  -> 异常：{e}")

    # 失败步骤挂上关键上下文，方便排查
    if result["status"] != "ok":
        from .browser import _api_tracker
        # API 调用记录
        result["api_trace"] = _api_tracker.get_apis()
        # 页面 URL
        result["page_url"] = browser.get_current_url()
        # 页面快照（截断）
        snapshot = state.get("page_snapshot", "")
        result["page_snapshot"] = snapshot[:500] if snapshot else "(无)"
        # 截图
        shot_name = f"step_{idx+1}_fail"
        await browser.screenshot(shot_name)
        result["screenshot"] = shot_name
        # 定位策略详情（click/fill 专用）
        if action in ("click", "fill"):
            result["locate_detail"] = dict(browser.last_locate_detail)

    state["history"].append(result)
    state["current_step"] = idx + 1

    return state


# ============================================================
# Analyzer：多层断言（代码级优先 → LLM 兜底）
# ============================================================

ASSERT_PROMPT = """你是测试断言器。根据当前页面内容，判断预期条件是否满足。

当前页面内容：
{snapshot}

预期条件：{expected}

请判断结果。只返回 JSON：
{{"passed": true/false, "reason": "简短说明"}}"""

# ---- 断言类型检测 ----

def _detect_assert_type(target: str) -> str:
    """根据 target 字符串判断断言类型，返回类型名或空字符串（走 LLM）"""
    t = target.strip()

    # 1. URL 匹配：以 "/" 开头，或以 "URL " 开头
    if t.startswith("/") or re.match(r'^URL\s+', t, re.IGNORECASE):
        return "url"

    # 2. API 状态码："API /path 返回 200"
    if re.match(r'^API\s+\S+\s+返回\s+\d{3}', t, re.IGNORECASE):
        return "api"

    # 3. 表格行数："table 货源列表 行数 >= 5"
    if re.match(r'^table\s+.+\s+行数\s*[><=!]', t, re.IGNORECASE):
        return "table_rows"

    # 4. 元素属性：元素名后跟 disabled/enabled/checked/visible/hidden
    #    例如 "提交" disabled, "checkbox1" checked
    if re.search(r'\s+(disabled|enabled|checked|visible|hidden)\s*$', t, re.IGNORECASE):
        return "element_attr"

    # 5. 关键词匹配（快照中查文本）
    return "keywords"


# ---- 分流入口 ----

async def _smart_assert(state: TestState, step: dict, result: dict) -> dict:
    """断言：按类型分流，代码级优先（0 token），解决不了再走 LLM"""
    target = step.get("target", step.get("description", ""))
    if not target:
        return await _llm_assert(state, step, result)

    assert_type = _detect_assert_type(target)
    print(f"       断言类型: {assert_type}")

    if assert_type == "url":
        return await _assert_url(state, step, result)
    elif assert_type == "api":
        return await _assert_api(state, step, result)
    elif assert_type == "table_rows":
        return await _assert_table_rows(state, step, result)
    elif assert_type == "element_attr":
        return await _assert_element_attr(state, step, result)
    else:
        return await _assert_keywords(state, step, result)


# ---- 0-token 记录 ----

def _record_zero_token(state: TestState):
    """记录一次 0 token 的代码断言"""
    _record_tokens(state, type("Dummy", (), {
        "response_metadata": {"token_usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}}
    }), "代码断言")


# ---- LLM 重定位（自愈兜底）----

RELOCATE_PROMPT = """你是一个元素定位助手。目标元素未能在页面上直接定位到，请分析当前页面快照，给出替代的定位策略。

当前页面快照（列出所有可交互元素及其 role）：
{snapshot}

目标元素描述：{target}
原始 role：{role}
原始 action：{action}

请分析快照内容，找出最可能匹配的元素。如果找到，返回新的定位信息。如果快照中确实不存在该元素，明确说明。

只返回 JSON：
{{"role": "建议的 aria role（如 textbox/button/link/combobox，空字符串表示放弃 role 匹配）",
  "name": "建议的元素名称/文本（用快照中实际出现的文字）",
  "reason": "简短分析说明"}}"""


async def _llm_relocate(state: TestState, step: dict) -> dict | None:
    """
    LLM 兜底：元素定位全部失败时，让 LLM 看快照重新定位。
    返回新的定位信息 dict 或 None（LLM 也找不到）。
    """
    target = step.get("target", "")
    role = step.get("role", "")
    action = step.get("action", "")
    # 强制刷新：拿到失败后的最新 DOM，同时更新缓存
    snapshot, _ = await _browser.get_page_state(force_refresh=True)
    state["page_snapshot"] = snapshot

    print(f"       [自愈] LLM 重定位 '{target}' ...")

    try:
        response = await executor_llm.ainvoke([
            SystemMessage(content=RELOCATE_PROMPT.format(
                snapshot=snapshot,
                target=target,
                role=role,
                action=action,
            )),
        ])
    except Exception as e:
        print(f"       [自愈] LLM 调用失败: {e}")
        return None

    _record_tokens(state, response, "LLM 重定位")

    try:
        parsed = parse_json_safely(response.content, label="Relocate 输出")
        if isinstance(parsed, list):
            parsed = parsed[0] if parsed else {}
    except Exception as e:
        print(f"       [自愈] LLM 返回解析失败: {e}")
        return None

    new_role = parsed.get("role", "").strip()
    new_name = parsed.get("name", "").strip()
    reason = parsed.get("reason", "")

    if not new_name:
        print(f"       [自愈] LLM 判定元素不存在: {reason}")
        return None

    print(f"       [自愈] LLM 建议: role='{new_role}' name='{new_name}' ({reason})")
    return {"role": new_role, "name": new_name}


# ---- 断言实现：URL 匹配 ----

async def _assert_url(state: TestState, step: dict, result: dict) -> dict:
    """代码断言：URL 匹配（如 assert "/dashboard"、assert "URL /login"）"""
    target = step.get("target", "").strip()
    if re.match(r'^URL\s+', target, re.IGNORECASE):
        target = re.sub(r'^URL\s+', '', target, flags=re.IGNORECASE).strip()

    current_url = _browser.get_current_url()
    passed = target in current_url

    _record_zero_token(state)
    result["status"] = "ok" if passed else "fail"
    result["message"] = f"URL 断言: '{target}' {'✓ 匹配' if passed else '✗ 不匹配'} (当前: {current_url})"
    print(f"       {'✓' if passed else '✗'} {result['message']}")
    return result


# ---- 断言实现：API 状态码 ----

def _parse_api_assert(target: str) -> tuple[str, int] | None:
    """解析 'API /api/login 返回 200' 格式，返回 (path, status_code) 或 None"""
    m = re.match(r'^API\s+(\S+)\s+返回\s+(\d{3})', target.strip(), re.IGNORECASE)
    if not m:
        return None
    return m.group(1), int(m.group(2))


async def _assert_api(state: TestState, step: dict, result: dict) -> dict:
    """代码断言：API 状态码（如 assert "API /api/login 返回 200"）"""
    target = step.get("target", "")
    parsed = _parse_api_assert(target)
    if not parsed:
        return await _llm_assert(state, step, result)

    api_path, expected_code = parsed

    from browser import _api_tracker
    apis = _api_tracker.get_apis()

    matched_api = None
    for api in apis:
        if api_path in api.get("url", ""):
            matched_api = api
            break

    _record_zero_token(state)

    if matched_api is None:
        recorded = [a.get("url", "") for a in apis]
        result["status"] = "fail"
        result["message"] = f"API 断言: '{api_path}' 未触发 (已触发: {recorded})"
        print(f"       ✗ {result['message']}")
        return result

    actual_code = matched_api.get("status", 0)
    passed = actual_code == expected_code
    result["status"] = "ok" if passed else "fail"
    result["message"] = f"API 断言: '{api_path}' 返回 {actual_code} {'✓' if passed else '✗ 期望 ' + str(expected_code)}"
    print(f"       {'✓' if passed else '✗'} {result['message']}")
    return result


# ---- 断言实现：表格行数 ----

def _parse_table_rows_assert(target: str) -> tuple[str, str, int] | None:
    """解析 'table 货源列表 行数 >= 5' 格式，返回 (table_name, operator, count) 或 None"""
    m = re.match(r'^table\s+(.+?)\s+行数\s*(>=|<=|>|<|==|!=)\s*(\d+)', target.strip(), re.IGNORECASE)
    if not m:
        return None
    return m.group(1).strip(), m.group(2), int(m.group(3))


async def _assert_table_rows(state: TestState, step: dict, result: dict) -> dict:
    """代码断言：表格行数（如 assert "table 货源列表 行数 >= 5"）"""
    target = step.get("target", "")
    parsed = _parse_table_rows_assert(target)
    if not parsed:
        return await _llm_assert(state, step, result)

    table_name, operator, expected = parsed

    actual = await _browser.count_table_rows(table_name)

    _record_zero_token(state)

    if actual < 0:
        result["status"] = "fail"
        result["message"] = f"表格断言: 未找到表格 '{table_name}'"
        print(f"       ✗ {result['message']}")
        return result

    # 执行比较
    ops = {
        ">=": lambda a, e: a >= e, "<=": lambda a, e: a <= e,
        ">":  lambda a, e: a > e,  "<":  lambda a, e: a < e,
        "==": lambda a, e: a == e, "!=": lambda a, e: a != e,
    }
    passed = ops[operator](actual, expected)

    result["status"] = "ok" if passed else "fail"
    result["message"] = f"表格断言: '{table_name}' 行数={actual} {operator} {expected} {'✓' if passed else '✗'}"
    print(f"       {'✓' if passed else '✗'} {result['message']}")
    return result


# ---- 断言实现：元素属性 ----

def _parse_element_attr_assert(target: str) -> tuple[str, str] | None:
    """解析 '提交 disabled' 格式，返回 (element_name, attribute) 或 None"""
    m = re.search(r'^(.+)\s+(disabled|enabled|checked|visible|hidden)\s*$', target.strip(), re.IGNORECASE)
    if not m:
        return None
    return m.group(1).strip(), m.group(2).lower()


async def _assert_element_attr(state: TestState, step: dict, result: dict) -> dict:
    """代码断言：元素属性（如 assert "提交" disabled）"""
    target = step.get("target", "")
    parsed = _parse_element_attr_assert(target)
    if not parsed:
        return await _llm_assert(state, step, result)

    element_name, attr = parsed

    found, msg = await _browser.check_element_attribute(element_name, attr)

    _record_zero_token(state)

    if not found:
        result["status"] = "fail"
        result["message"] = msg
        print(f"       ✗ {result['message']}")
        return result

    # 解析 check_element_attribute 返回的消息来判断通过/失败
    # msg 格式: "'名称' disabled=True" 或 "'名称' enabled=False"
    import ast
    try:
        # msg 中提取 bool 值
        attr_value_str = msg.rsplit("=", 1)[-1].strip()
        attr_value = ast.literal_eval(attr_value_str)
    except Exception:
        attr_value = None

    # enabled 是反向逻辑（disabled 的反面）
    if attr == "enabled":
        passed = attr_value is True
        result["status"] = "ok" if passed else "fail"
        result["message"] = f"元素属性: '{element_name}' {'可用' if passed else '不可用（disabled）'}"
    elif attr in ("disabled", "checked", "visible"):
        passed = attr_value is True
        result["status"] = "ok" if passed else "fail"
        label = {"disabled": "已禁用", "checked": "已勾选", "visible": "可见"}[attr]
        result["message"] = f"元素属性: '{element_name}' {label if passed else ('未' + label)}"
    elif attr == "hidden":
        passed = attr_value is True
        result["status"] = "ok" if passed else "fail"
        result["message"] = f"元素属性: '{element_name}' {'不可见' if passed else '可见'}"
    else:
        passed = False
        result["status"] = "fail"
        result["message"] = f"元素属性: 未知属性 '{attr}'"

    print(f"       {'✓' if passed else '✗'} {result['message']}")
    return result


# ---- 断言实现：关键词匹配（原逻辑）----

async def _assert_keywords(state: TestState, step: dict, result: dict) -> dict:
    """代码断言：快照关键词匹配（0 token），有关键词不匹配降级到 LLM"""
    target = step.get("target", step.get("description", ""))
    snapshot = state.get("page_snapshot", "")

    if not snapshot:
        snapshot, _ = await _browser.get_page_state()

    keywords = [k.strip() for k in re.split(r"[，,\s]+", target) if k.strip()]

    snapshot_lower = snapshot.lower()
    for kw in keywords:
        found = kw.lower() in snapshot_lower
        print(f"       代码断言(关键词): '{kw}' {'✓ 找到' if found else '✗ 未找到'}")
        if not found:
            print(f"       → 降级到 LLM 断言")
            return await _llm_assert(state, step, result)

    _record_zero_token(state)
    result["status"] = "ok"
    result["message"] = f"页面中包含 '{target}'（代码断言）"
    return result


# ---- 兜底：LLM 断言 ----

async def _llm_assert(state: TestState, step: dict, result: dict) -> dict:
    """用 LLM 断言，含返回值校验"""
    snapshot = state.get("page_snapshot", "")

    if not snapshot:
        snapshot, _ = await _browser.get_page_state()

    response = await executor_llm.ainvoke([
        SystemMessage(content=ASSERT_PROMPT.format(
            snapshot=snapshot,
            expected=step.get("target", step.get("description", "")),
        )),
    ])

    _record_tokens(state, response, "LLM 断言")

    raw_text = response.content

    # 安全解析 + 校验
    parsed = parse_json_safely(raw_text, label="Assert 输出")
    if isinstance(parsed, list):
        parsed = parsed[0] if parsed else {}

    passed, reason = validate_assert_result(parsed)

    result["status"] = "ok" if passed else "fail"
    result["message"] = reason
    print(f"  -> 断言：{'✓' if passed else '✗'} {reason}")

    return result


# ============================================================
# Router：决定下一步
# ============================================================

def router(state: TestState) -> Literal["execute", "report"]:
    """还有步骤 → 继续执行；完成 → 生成报告"""
    if state.get("error"):
        return "report"
    if state["current_step"] < len(state["steps"]):
        return "execute"
    return "report"


# ============================================================
# Reporter 节点
# ============================================================

async def reporter(state: TestState, browser: BrowserSession) -> TestState:
    """生成测试报告"""
    print("\n[Reporter] 生成测试报告...")

    passed = all(h["status"] == "ok" for h in state["history"])
    failed_steps = [h for h in state["history"] if h["status"] != "ok"]

    lines = [
        "=" * 50,
        "  自动化测试报告",
        "=" * 50,
        f"测试场景：{state['test_scenario']}",
        f"目标 URL：{state['target_url']}",
        f"总步骤数：{len(state['steps'])}",
        f"结果：{'✓ 全部通过' if passed else '✗ 存在失败'}",
        "",
    ]

    for h in state["history"]:
        status = h["status"]
        if status == "ok":
            icon = "✓"
        elif status == "crash":
            icon = "💥"
        else:
            icon = "✗"
        lines.append(f"  {icon} 步骤 {h['step']}: [{h['action']}] {h['target']}")
        if h["message"]:
            lines.append(f"      {h['message']}")
        # 失败步骤的上下文信息
        if status != "ok":
            if h.get("page_url"):
                lines.append(f"      页面: {h['page_url']}")
            if h.get("locate_detail"):
                ld = h["locate_detail"]
                lines.append(f"      定位策略: {' → '.join(ld.get('tried', []))} → 全部失败")
            if h.get("screenshot"):
                lines.append(f"      截图: screenshots/{h['screenshot']}.png")
            if h.get("page_snapshot"):
                lines.append(f"      快照: {h['page_snapshot'][:200]}...")
        # 显示失败的 API 调用
        for a in h.get("api_trace", []):
            if a.get("status", 0) >= 400:
                body = a.get("body", "")
                if body:
                    try:
                        body = str(json.loads(body))[:200]
                    except Exception:
                        body = body[:200]
                lines.append(f"      API错误: {a.get('method', '?')} {a.get('url', '?')} → {a.get('status', '?')} {body}")
        # 自愈标记
        if h.get("_self_healed"):
            lines.append(f"      ♻ 已通过自愈重试恢复")

    # 崩溃汇总
    crash_steps = [h for h in state["history"] if h.get("status") == "crash"]
    if crash_steps:
        lines.append("")
        lines.append(f"⚠ 测试因浏览器崩溃而中断，未执行步骤: {len(state['steps']) - len(state['history'])}")

    if failed_steps:
        lines.append("")
        lines.append(f"失败步骤数：{len(failed_steps)}")

    # Token 消耗统计
    u = state.get("token_usage", {})
    if u.get("total", 0) > 0:
        lines.append("")
        lines.append("-" * 50)
        lines.append("Token 消耗统计：")
        lines.append(f"  总计：{u['total']}")
        lines.append(f"  输入（prompt）：{u['prompt']}")
        lines.append(f"  输出（completion）：{u['completion']}")
        for d in u.get("details", []):
            lines.append(f"  ─ [{d['label']}] prompt={d['prompt']} completion={d['completion']}")

    lines.append("=" * 50)

    report = "\n".join(lines)
    state["report"] = report

    await browser.screenshot("final")

    print(report)
    return state


# ============================================================
# Graph 构建
# ============================================================

# 用闭包把 browser 注入节点（LangGraph 节点的函数签名必须一致）

_browser: BrowserSession = None  # type: ignore


def _make_planner():
    async def fn(state: TestState):
        return await planner(state, _browser)
    return fn


def _make_executor():
    async def fn(state: TestState):
        return await execute_step(state, _browser)
    return fn


def _make_reporter():
    async def fn(state: TestState):
        return await reporter(state, _browser)
    return fn


def build_graph() -> StateGraph:
    """构建 LangGraph 工作流"""
    workflow = StateGraph(TestState)

    workflow.add_node("planner", _make_planner())
    workflow.add_node("executor", _make_executor())
    workflow.add_node("reporter", _make_reporter())

    workflow.set_entry_point("planner")
    workflow.add_edge("planner", "executor")
    workflow.add_conditional_edges(
        "executor",
        router,
        {"execute": "executor", "report": "reporter"},
    )
    workflow.add_edge("reporter", END)

    return workflow.compile()


# ============================================================
# 运行入口
# ============================================================

async def run_test(
    browser_session: BrowserSession,
    target_url: str,
    test_scenario: str,
) -> tuple[str, bool]:
    """运行一次完整测试，返回 (报告文本, 是否全部通过)"""
    global _browser
    _browser = browser_session

    graph = build_graph()

    initial_state: TestState = {
        "target_url": target_url,
        "test_scenario": test_scenario,
        "steps": [],
        "current_step": 0,
        "page_snapshot": "",
        "is_cached": False,
        "history": [],
        "report": "",
        "error": None,
        "token_usage": {"total": 0, "prompt": 0, "completion": 0, "details": []},
    }

    final_state = await graph.ainvoke(initial_state)
    report = final_state["report"]
    passed = all(h["status"] == "ok" for h in final_state.get("history", []))
    return report, passed
