"""Round-trip one structured UI scenario per XLSX workbook.

The hidden replay sheet and SHA-256 check are adapted from
laya-pilot-main/lib/workflow_excel.py. The digest checks consistency only.

MIT License

Copyright (c) 2026 LayaPilot contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
"""

import hashlib
import json
from pathlib import Path

from openpyxl import Workbook, load_workbook


_STEPS_SHEET = "测试步骤"
_REPLAY_SHEET = "__scenario_v1__"
_HEADERS = ("序号", "步骤 JSON")
_REQUIRED_FIELDS = {
    "navigate": ("url",),
    "click": ("role", "name"),
    "fill": ("role", "name", "data_key"),
    "wait_text": ("text",),
    "assert_text": ("text",),
    "assert_url": ("expected",),
}


def export_case_workbook(path: str | Path, scenario: dict) -> None:
    """Write a workbook from a platform scenario JSON object."""
    rows = _step_rows(scenario)
    payload = _encode(scenario)
    if len(payload) > 32767:
        raise ValueError("场景 JSON 超过 Excel 单元格长度限制")

    book = Workbook()
    try:
        sheet = book.active
        sheet.title = _STEPS_SHEET
        sheet.freeze_panes = "A2"
        sheet.column_dimensions["B"].width = 90
        sheet.append(_HEADERS)
        for row in rows:
            sheet.append(row)
        replay = book.create_sheet(_REPLAY_SHEET)
        replay.sheet_state = "veryHidden"
        replay.append([payload, _digest(payload)])
        book.save(path)
    finally:
        book.close()


def import_case_workbook(path: str | Path) -> dict:
    """Return the scenario after checking the hidden payload and visible rows."""
    book = load_workbook(path, read_only=True)
    try:
        if _STEPS_SHEET not in book or _REPLAY_SHEET not in book:
            raise ValueError("工作簿缺少结构化 UI 回放所需的工作表")
        replay = book[_REPLAY_SHEET]
        payload, digest = replay["A1"].value, replay["B1"].value
        if (replay.sheet_state not in {"hidden", "veryHidden"}
                or not isinstance(payload, str) or _digest(payload) != digest):
            raise ValueError("隐藏回放数据不完整或摘要不一致")
        scenario = json.loads(payload)
        expected = [_HEADERS, *_step_rows(scenario)]
        if list(book[_STEPS_SHEET].values) != expected:
            raise ValueError("可见步骤与隐藏回放数据不一致")
        return scenario
    finally:
        book.close()


def _step_rows(scenario: dict) -> list[tuple[int, str]]:
    steps = scenario.get("steps") if isinstance(scenario, dict) else None
    if not isinstance(steps, list) or not steps:
        raise ValueError("结构化 UI 用例必须至少包含一个步骤")
    rows = []
    for number, step in enumerate(steps, 1):
        action = step.get("action") if isinstance(step, dict) else None
        if not isinstance(action, str) or action not in _REQUIRED_FIELDS:
            raise ValueError(f"第 {number} 步包含不支持的动作")
        if any(not isinstance(step.get(key), str) or not step[key].strip()
               for key in _REQUIRED_FIELDS[action]):
            raise ValueError(f"第 {number} 步 {action} 缺少必填字段")
        if action == "assert_url" and step.get("match", "contains") not in ("contains", "equals", "exact"):
            raise ValueError(f"第 {number} 步 URL 匹配模式无效")
        rows.append((number, _encode(step)))
    return rows


def _encode(value: dict) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False)


def _digest(payload: str) -> str:
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()
