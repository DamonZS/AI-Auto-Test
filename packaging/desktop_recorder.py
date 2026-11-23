import asyncio
import json
from concurrent.futures import Future
from pathlib import Path

from PySide6.QtCore import QStandardPaths, QThread, Signal
from PySide6.QtWidgets import QDialog, QFileDialog, QFormLayout, QHBoxLayout, QLabel, QLineEdit, QPushButton
from playwright.async_api import Error

from agents.recorder import record_ui_case


class RecordingThread(QThread):
    ready = Signal()

    def __init__(self, url, parent):
        super().__init__(parent)
        self.url = url
        self.decision = Future()
        self.draft = None
        self.error = ""

    def stop(self, export):
        if not self.decision.done():
            self.decision.set_result(export)

    def run(self):
        try:
            self.draft = asyncio.run(self.record())
        except ValueError as problem:
            self.error = str(problem)
        except (Error, OSError):
            self.error = "Chromium 录制失败，请检查目标页面和浏览器安装。"

    async def record(self):
        decision = asyncio.wrap_future(self.decision)

        async def wait_for_stop(page):
            if not page.is_closed():
                closed = asyncio.get_running_loop().create_future()
                page.once("close", lambda _: closed.set_result(None))
                self.ready.emit()
                await asyncio.wait((closed, decision), return_when=asyncio.FIRST_COMPLETED)
            return not decision.done() or decision.result()

        if decision.done() and not decision.result():
            return None
        return await record_ui_case(self.url, wait_for_stop, startup_decision=decision)


class RecorderDialog(QDialog):
    def __init__(self, parent):
        super().__init__(parent)
        self.setWindowTitle("录制用例")
        self.setMinimumWidth(480)
        self.worker = None
        self.draft = None
        self.discarding = False
        self.export_requested = False

        self.address = QLineEdit(self)
        self.address.setPlaceholderText("https://")
        self.address.returnPressed.connect(self.start_recording)
        self.status = QLabel(self)
        self.status.setWordWrap(True)
        self.start_button = QPushButton("开始录制", self)
        self.start_button.clicked.connect(self.start_recording)
        self.export_button = QPushButton("停止并导出", self)
        self.export_button.setEnabled(False)
        self.export_button.clicked.connect(self.export_recording)
        self.discard_button = QPushButton("放弃", self)
        self.discard_button.clicked.connect(self.reject)

        buttons = QHBoxLayout()
        buttons.addStretch()
        for button in (self.start_button, self.export_button, self.discard_button):
            buttons.addWidget(button)
        layout = QFormLayout(self)
        layout.addRow("页面地址", self.address)
        layout.addRow(self.status)
        layout.addRow(buttons)

    def start_recording(self):
        if not self.start_button.isEnabled():
            return
        self.start_button.setEnabled(False)
        self.status.setText("正在打开录制浏览器…")
        self.worker = RecordingThread(self.address.text().strip(), self)
        self.address.clear()
        self.address.setEnabled(False)
        self.worker.ready.connect(self.recording_ready)
        self.worker.finished.connect(self.recording_finished)
        self.worker.start()

    def recording_ready(self):
        if not self.discarding:
            self.export_button.setEnabled(True)
            self.status.setText("录制中，请在独立浏览器中操作。")

    def recording_finished(self):
        worker, self.worker = self.worker, None
        worker.deleteLater()
        if self.discarding:
            self.reject()
        elif worker.draft and worker.draft["steps"]:
            self.draft = worker.draft
            self.status.setText("录制已结束，可导出步骤 JSON。")
            self.export_button.setText("导出 JSON")
            self.export_button.setEnabled(True)
            if self.export_requested:
                self.export_recording()
        else:
            self.status.setText(worker.error or "没有可导出的步骤。")
            self.address.setEnabled(True)
            self.start_button.setEnabled(True)

    def export_recording(self):
        if self.worker is not None:
            self.export_requested = True
            self.export_button.setEnabled(False)
            self.status.setText("正在结束录制…")
            self.worker.stop(True)
            return
        documents = QStandardPaths.writableLocation(QStandardPaths.StandardLocation.DocumentsLocation)
        suggested = str(Path(documents) / "录制草稿.json")
        destination, _ = QFileDialog.getSaveFileName(self, "导出录制草稿", suggested, "JSON 文件 (*.json)")
        if not destination:
            return
        path = Path(destination)
        try:
            path.write_text(json.dumps(self.draft, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        except OSError:
            self.status.setText("无法保存录制草稿，请选择可写入的位置。")
            return
        self.reject()

    def reject(self):
        if self.worker is not None:
            self.discarding = True
            self.export_button.setEnabled(False)
            self.discard_button.setEnabled(False)
            self.status.setText("正在关闭录制浏览器…")
            self.worker.stop(False)
            return
        self.draft = None
        self.discarding = False
        self.export_requested = False
        self.status.clear()
        self.address.setEnabled(True)
        self.start_button.setEnabled(True)
        self.export_button.setText("停止并导出")
        self.export_button.setEnabled(False)
        self.discard_button.setEnabled(True)
        super().reject()

    def shutdown(self):
        if self.worker is not None:
            self.discarding = True
            self.worker.stop(False)
            self.worker.wait()
        self.draft = None
