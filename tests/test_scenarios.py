"""旧版 Excel 自然语言联调。设置 AUTO_AI_E2E_WORKBOOK 为工作簿路径后启用。"""

import os

import pytest

from auto_ai.excel_reader import cases_for_pytest


workbook = os.environ.get("AUTO_AI_E2E_WORKBOOK")
if workbook:
    ids, values = cases_for_pytest(workbook)
else:
    ids = ["disabled"]
    values = [pytest.param(None, None, marks=pytest.mark.skip(
        reason="设置 AUTO_AI_E2E_WORKBOOK 后才启用真实站点和模型联调"
    ))]


@pytest.mark.parametrize("url,scenario", values, ids=ids)
async def test_from_excel(session, url, scenario):
    from auto_ai.agent import run_test

    report, passed = await run_test(session, url, scenario)
    assert passed, f"\n{report}"
