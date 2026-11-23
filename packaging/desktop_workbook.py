import json
import re
from pathlib import Path
from zipfile import BadZipFile

from openpyxl import load_workbook
from openpyxl.utils.exceptions import InvalidFileException
from PySide6.QtCore import QFile, QIODevice, QObject, QStandardPaths, Slot
from PySide6.QtWebChannel import QWebChannel
from PySide6.QtWebEngineCore import QWebEngineScript
from PySide6.QtWidgets import QFileDialog

from agents.case_workbook import export_case_workbook, import_case_workbook
from auto_ai.excel_reader import load_cases


_XLSX_FILTER = "Excel 工作簿 (*.xlsx)"


class CaseWorkbookBridge(QObject):
    @Slot(result="QVariantMap")
    def importWorkbook(self):
        path, _ = QFileDialog.getOpenFileName(self.parent(), "导入用例工作簿", "", _XLSX_FILTER)
        if not path:
            return {"cancelled": True}
        try:
            book = load_workbook(path, read_only=True)
            try:
                structured = any(name in book.sheetnames for name in ("测试步骤", "__scenario_v1__"))
            finally:
                book.close()
            if structured:
                return {"name": Path(path).stem, "scenario": import_case_workbook(path)}
            return {"legacyCases": load_cases(path)}
        except ValueError as error:
            return {"error": str(error)}
        except (BadZipFile, InvalidFileException, KeyError, OSError, SyntaxError):
            return {"error": "无法读取工作簿，请选择有效的 .xlsx 文件。"}

    @Slot(str, str, result="QVariantMap")
    def exportWorkbook(self, name, scenario_json):
        try:
            scenario = json.loads(scenario_json)
        except ValueError:
            return {"error": "仅支持导出结构化 UI 用例。"}

        filename = re.sub(r'[<>:"/\\|?*\x00-\x1f]', "_", name).strip(" .") or "用例"
        if not filename.lower().endswith(".xlsx"):
            filename += ".xlsx"
        documents = QStandardPaths.writableLocation(QStandardPaths.StandardLocation.DocumentsLocation)
        dialog = QFileDialog(self.parent(), "导出用例工作簿", documents, _XLSX_FILTER)
        dialog.setAcceptMode(QFileDialog.AcceptMode.AcceptSave)
        dialog.setDefaultSuffix("xlsx")
        dialog.selectFile(filename)
        if dialog.exec() != QFileDialog.DialogCode.Accepted:
            return {"cancelled": True}

        path = Path(dialog.selectedFiles()[0])
        if path.suffix.lower() != ".xlsx":
            return {"error": "请使用 .xlsx 文件扩展名。"}
        try:
            export_case_workbook(path, scenario)
        except ValueError as error:
            return {"error": str(error)}
        except OSError:
            return {"error": "无法保存工作簿，请选择可写入的位置。"}
        return {"saved": True}


def install_case_workbook_bridge(page, window):
    channel = QWebChannel(page)
    channel.registerObject("caseWorkbook", CaseWorkbookBridge(window))
    page.setWebChannel(channel)

    library = QFile(":/qtwebchannel/qwebchannel.js")
    if not library.open(QIODevice.OpenModeFlag.ReadOnly):
        raise RuntimeError("无法加载 Qt WebChannel 脚本")
    source = bytes(library.readAll()).decode("utf-8")
    library.close()

    script = QWebEngineScript()
    script.setName("case-workbook")
    script.setInjectionPoint(QWebEngineScript.InjectionPoint.DocumentCreation)
    script.setWorldId(QWebEngineScript.ScriptWorldId.MainWorld)
    script.setSourceCode(source + """
;if (window.qt?.webChannelTransport) {
  new QWebChannel(qt.webChannelTransport, (channel) => {
    window.caseWorkbook = channel.objects.caseWorkbook;
    window.dispatchEvent(new Event('case-workbook-ready'));
  });
}
""")
    page.scripts().insert(script)
