import pytest
from openpyxl import load_workbook

from agents.case_workbook import export_case_workbook, import_case_workbook


@pytest.fixture
def scenario():
    return {"steps": [
        {"action": "navigate", "url": "https://example.test/"},
        {"action": "fill", "role": "textbox", "name": "账号", "data_key": "account"},
        {"action": "click", "role": "button", "name": "登录"},
        {"action": "wait_text", "text": "欢迎"},
        {"action": "assert_text", "text": "=1+1\n完成"},
        {"action": "assert_url", "expected": "/home", "match": "contains"},
    ]}


def test_round_trip_preserves_replay_steps(tmp_path, scenario):
    path = tmp_path / "case.xlsx"
    export_case_workbook(path, scenario)

    assert import_case_workbook(path) == scenario
    book = load_workbook(path)
    try:
        assert book["__scenario_v1__"].sheet_state == "veryHidden"
        assert book["测试步骤"]["B6"].data_type == "s"
        assert '"data_key":"account"' in book["测试步骤"]["B3"].value
    finally:
        book.close()


@pytest.mark.parametrize("change", ["visible", "hidden", "missing", "reorder", "extra"])
def test_import_rejects_inconsistent_replay(tmp_path, scenario, change):
    path = tmp_path / "case.xlsx"
    export_case_workbook(path, scenario)
    book = load_workbook(path)
    sheet = book["测试步骤"]
    if change == "visible":
        sheet["B2"] = '=1+1'
    elif change == "hidden":
        meta = book["__scenario_v1__"]
        meta["A1"] = meta["A1"].value.replace("example.test", "other.test")
    elif change == "missing":
        del book["__scenario_v1__"]
    elif change == "reorder":
        sheet.move_range("A2:B2", rows=1)
    else:
        sheet.append([7, sheet["B2"].value])
    book.save(path)
    book.close()

    with pytest.raises(ValueError, match="回放"):
        import_case_workbook(path)


@pytest.mark.parametrize("step", [
    {"action": "delete"},
    {"action": "fill", "role": "textbox", "name": "账号", "value": "fixture"},
    {"action": "assert_url", "expected": "/home", "match": "regex"},
])
def test_export_rejects_invalid_steps(tmp_path, step):
    path = tmp_path / "case.xlsx"
    with pytest.raises(ValueError):
        export_case_workbook(path, {"steps": [step]})
    assert not path.exists()


def test_export_rejects_excel_truncation(tmp_path, scenario):
    scenario["steps"][0]["url"] += "x" * 32767
    with pytest.raises(ValueError, match="单元格"):
        export_case_workbook(tmp_path / "case.xlsx", scenario)
