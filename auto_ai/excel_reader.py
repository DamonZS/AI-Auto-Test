"""Excel 用例读取工具 —— 从 .xlsx 文件中加载测试用例"""

from pathlib import Path
from typing import Optional


def load_cases(
    filepath: str,
    sheet: str | int = 0,
) -> list[dict]:
    """
    从 Excel 文件读取测试用例。

    Excel 表头（大小写不敏感）：
        url (必填)     — 目标 URL
        scenario (必填) — 测试场景（自然语言描述）
        name (可选)     — 用例名称（用于报告展示，不填则用 scenario 前 30 字）

    返回: [{"url": "...", "scenario": "...", "name": "..."}, ...]

    示例:
        >>> cases = load_cases("tests/cases/login.xlsx")
        >>> for c in cases:
        ...     report, passed = await run_test(session, c["url"], c["scenario"])
    """
    try:
        import openpyxl
    except ImportError:
        raise ImportError(
            "读取 Excel 需要 openpyxl 库，请执行: pip install openpyxl"
        )

    filepath = Path(filepath)
    if not filepath.exists():
        raise FileNotFoundError(f"用例文件不存在: {filepath.resolve()}")

    if filepath.suffix.lower() not in (".xlsx",):
        raise ValueError(f"仅支持 .xlsx 格式，当前文件: {filepath.name}")

    wb = openpyxl.load_workbook(filepath, read_only=True, data_only=True)

    # 选择 sheet
    if isinstance(sheet, int):
        if sheet >= len(wb.sheetnames):
            raise ValueError(
                f"Sheet 索引 {sheet} 超出范围，可用: {wb.sheetnames}"
            )
        ws = wb[wb.sheetnames[sheet]]
    else:
        if sheet not in wb.sheetnames:
            raise ValueError(
                f"Sheet '{sheet}' 不存在，可用: {wb.sheetnames}"
            )
        ws = wb[sheet]

    # 读取表头（第一行）
    headers = []
    for cell in ws[1]:
        if cell.value is not None:
            headers.append(str(cell.value).strip().lower())

    # 映射列索引
    def _find_col(*aliases: str) -> Optional[int]:
        for idx, h in enumerate(headers):
            if h in aliases:
                return idx
        return None

    url_col = _find_col("url")
    scenario_col = _find_col("scenario", "场景", "用例", "case", "测试场景")
    name_col = _find_col("name", "名称", "用例名", "description", "描述", "说明")

    if url_col is None:
        raise ValueError(f"Excel 缺少 'url' 列，当前表头: {headers}")
    if scenario_col is None:
        raise ValueError(f"Excel 缺少 'scenario' 列，当前表头: {headers}")

    # 读取数据行
    result = []
    for row in ws.iter_rows(min_row=2, values_only=True):
        url = row[url_col] if url_col < len(row) else None
        scenario = row[scenario_col] if scenario_col < len(row) else None
        name = None
        if name_col is not None and name_col < len(row):
            name = row[name_col]

        # 跳过空行
        if url is None or scenario is None:
            continue
        url = str(url).strip()
        scenario = str(scenario).strip()
        if not url or not scenario:
            continue

        case = {
            "url": url,
            "scenario": scenario,
            "name": str(name).strip() if name else scenario[:30],
        }
        result.append(case)

    wb.close()

    if not result:
        raise ValueError(
            f"Excel 文件中没有有效的用例数据（url + scenario 都非空），"
            f"请检查 {filepath.name} 的表头和内容。"
        )

    return result


# ---------- pytest parametrize 辅助 ----------

def cases_for_pytest(filepath: str, sheet: str | int = 0):
    """
    返回 pytest.mark.parametrize 所需的 (ids, argvalues) 元组。

    用法:
        import pytest
        from auto_ai.excel_reader import cases_for_pytest

        ids, values = cases_for_pytest("tests/cases/login.xlsx")

        @pytest.mark.parametrize("url,scenario", values, ids=ids)
        async def test_login(session, url, scenario):
            report, passed = await run_test(session, url, scenario)
            assert passed, f"\n{report}"
    """
    cases = load_cases(filepath, sheet)
    ids = [c["name"] for c in cases]
    values = [(c["url"], c["scenario"]) for c in cases]
    return ids, values
