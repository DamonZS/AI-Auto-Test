import re
from collections.abc import Iterable, Mapping


_ROLE_SELECTOR = re.compile(r'role=([a-z][a-z0-9-]*)\[name="(.*)"\]', re.DOTALL)


def convert_recorded_steps(events: Iterable[Mapping[str, object]]) -> dict:
    """Convert enabled recorder events to a structured UI scenario."""
    steps = []
    for index, event in enumerate(events, start=1):
        if not isinstance(event, Mapping):
            raise ValueError(f"第 {index} 条录制事件必须是对象")
        if event.get("enabled") is False:
            continue
        try:
            steps.append(_convert_step(event))
        except ValueError as error:
            raise ValueError(f"第 {index} 条录制事件：{error}") from None
    if not steps:
        raise ValueError("结构化 UI 用例必须至少包含一个步骤")
    return {"steps": steps}


def _convert_step(event: Mapping[str, object]) -> dict[str, str]:
    action = event.get("action")
    if action in ("goto", "navigate"):
        return {"action": "navigate", "url": _required_text(event, "url")}
    if action not in ("click", "fill"):
        raise ValueError("平台不支持该录制动作")
    if action == "click" and event.get("button", "") not in ("", "left"):
        raise ValueError("平台只支持左键点击")

    selector = _required_text(event, "selector")
    target = _ROLE_SELECTOR.fullmatch(selector)
    if not target or not target[2].strip():
        raise ValueError("需要带可访问名称的 role 选择器")
    role, name = target.groups()
    step = {"action": action, "role": role, "name": name.replace(r'\"', '"')}
    if action == "fill":
        step["data_key"] = _required_text(event, "data_key")
    return step


def _required_text(event: Mapping[str, object], field: str) -> str:
    value = event.get(field)
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"缺少有效的 {field}")
    return value
