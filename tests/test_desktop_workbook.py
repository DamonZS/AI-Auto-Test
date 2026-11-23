import json
from pathlib import Path

import pytest
from openpyxl import Workbook, load_workbook

from agents.case_workbook import export_case_workbook, import_case_workbook


@pytest.fixture
def workbook_window(monkeypatch):
    pytest.importorskip("PySide6.QtWebEngineCore")
    from PySide6.QtWidgets import QApplication, QWidget

    monkeypatch.syspath_prepend(str(Path(__file__).resolve().parents[1] / "packaging"))
    app = QApplication.instance() or QApplication(["Auto-Test"])
    window = QWidget()
    yield window
    window.close()
    app.processEvents()


@pytest.fixture
def workbook_bridge(workbook_window):
    from desktop_workbook import CaseWorkbookBridge

    return CaseWorkbookBridge(workbook_window)


def test_workbook_channel_injects_and_calls_python(tmp_path, monkeypatch, workbook_window):
    from PySide6.QtCore import QEventLoop, QTimer
    from PySide6.QtWebEngineCore import QWebEnginePage
    from PySide6.QtWidgets import QFileDialog
    from desktop_workbook import install_case_workbook_bridge

    source = tmp_path / "登录.xlsx"
    scenario = {"steps": [{"action": "assert_text", "text": "登录成功"}]}
    export_case_workbook(source, scenario)
    monkeypatch.setattr(QFileDialog, "getOpenFileName", lambda *args: (str(source), ""))
    page = QWebEnginePage(workbook_window)
    install_case_workbook_bridge(page, workbook_window)

    loop = QEventLoop()
    timer = QTimer(workbook_window)
    timer.setSingleShot(True)
    timer.timeout.connect(loop.quit)
    page.titleChanged.connect(lambda title: loop.quit() if title.startswith("{") else None)
    page.setHtml("""<script>
      window.addEventListener('case-workbook-ready', () => {
        window.caseWorkbook.importWorkbook((result) => document.title = JSON.stringify(result));
      });
    </script>""")
    timer.start(5000)
    loop.exec()
    timer.stop()

    assert json.loads(page.title()) == {"name": "登录", "scenario": scenario}


def test_desktop_bridge_imports_and_exports_cli_workbooks(tmp_path, monkeypatch, workbook_bridge):
    from PySide6.QtWidgets import QFileDialog

    scenario = {"steps": [{"action": "fill", "role": "textbox", "name": "用户", "data_key": "user"}]}
    source = tmp_path / "登录.xlsx"
    export_case_workbook(source, scenario)
    monkeypatch.setattr(QFileDialog, "getOpenFileName", lambda *args: (str(source), ""))
    assert workbook_bridge.importWorkbook() == {"name": "登录", "scenario": scenario}

    destination = tmp_path / "导出.xlsx"
    monkeypatch.setattr(QFileDialog, "exec", lambda self: QFileDialog.DialogCode.Accepted)
    monkeypatch.setattr(QFileDialog, "selectedFiles", lambda self: [str(destination)])
    assert workbook_bridge.exportWorkbook("登录", json.dumps(scenario)) == {"saved": True}
    assert import_case_workbook(destination) == scenario


def test_desktop_bridge_imports_legacy_cases(tmp_path, monkeypatch, workbook_bridge):
    from PySide6.QtWidgets import QFileDialog

    book = Workbook()
    book.active.append(["url", "scenario", "name"])
    book.active.append(["https://example.test/login", "检查登录界面", "登录"])
    book.active.append(["https://example.test/help", "检查帮助是否可用"])
    book.create_sheet("别的工作表").append(["url", "scenario", "name"])
    path = tmp_path / "legacy.xlsx"
    book.save(path)
    book.close()
    monkeypatch.setattr(QFileDialog, "getOpenFileName", lambda *args: (str(path), ""))

    assert workbook_bridge.importWorkbook() == {"legacyCases": [
        {"name": "登录", "scenario": "检查登录界面", "url": "https://example.test/login"},
        {"name": "检查帮助是否可用", "scenario": "检查帮助是否可用", "url": "https://example.test/help"},
    ]}


def test_desktop_bridge_rejects_tampering_and_cancels(tmp_path, monkeypatch, workbook_bridge):
    from PySide6.QtWidgets import QFileDialog

    path = tmp_path / "changed.xlsx"
    export_case_workbook(path, {"steps": [{"action": "assert_text", "text": "通过"}]})
    book = load_workbook(path)
    book["测试步骤"]["B2"] = "modified"
    book.save(path)
    book.close()
    monkeypatch.setattr(QFileDialog, "getOpenFileName", lambda *args: (str(path), ""))
    assert "不一致" in workbook_bridge.importWorkbook()["error"]

    path.write_bytes(b"invalid XLSX")
    assert "error" in workbook_bridge.importWorkbook()

    monkeypatch.setattr(QFileDialog, "getOpenFileName", lambda *args: ("", ""))
    assert workbook_bridge.importWorkbook() == {"cancelled": True}
    monkeypatch.setattr(QFileDialog, "exec", lambda self: QFileDialog.DialogCode.Rejected)
    assert workbook_bridge.exportWorkbook("test", '{"steps":[{"action":"assert_text","text":"ok"}]}') == {
        "cancelled": True,
    }
