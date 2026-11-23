import sys
from pathlib import Path

from PySide6.QtCore import QTimer, QUrl
from PySide6.QtGui import QIcon
from PySide6.QtWebEngineCore import QWebEnginePage, QWebEngineSettings
from PySide6.QtWebEngineWidgets import QWebEngineView
from PySide6.QtWidgets import QApplication, QFileDialog, QMainWindow, QMenu, QSystemTrayIcon

from desktop_recorder import RecorderDialog
from desktop_workbook import install_case_workbook_bridge


class WorkbenchPage(QWebEnginePage):
    def __init__(self, url, parent):
        super().__init__(parent)
        self.url = url
        self.settings().setAttribute(QWebEngineSettings.WebAttribute.JavascriptCanOpenWindows, False)

    def acceptNavigationRequest(self, url, navigation_type, is_main_frame):
        return not is_main_frame or (
            url.scheme() == self.url.scheme() and url.host() == self.url.host()
            and url.port() == self.url.port()
        )


class DesktopWindow(QMainWindow):
    def __init__(self, app, icon, url):
        super().__init__()
        self.setWindowTitle("Auto-Test")
        self.resize(1280, 820)
        self.setMinimumSize(800, 560)
        self.setWindowIcon(icon)

        browser = QWebEngineView(self)
        browser.setPage(WorkbenchPage(url, browser))
        install_case_workbook_bridge(browser.page(), self)
        browser.page().profile().downloadRequested.connect(self.save_download)
        self.setCentralWidget(browser)
        browser.setUrl(url)

        self.recorder = RecorderDialog(self)
        app.aboutToQuit.connect(self.recorder.shutdown)

        self.tray = QSystemTrayIcon(icon, self)
        self.tray.setToolTip("Auto-Test")
        menu = QMenu(self)
        menu.addAction("打开 Auto-Test", self.bring_forward)
        menu.addAction("退出 Auto-Test", app.quit)
        self.tray.setContextMenu(menu)
        self.tray.activated.connect(self.activate_from_tray)
        self.tray.show()

    def show_recorder(self):
        self.recorder.show()
        self.recorder.raise_()
        self.recorder.activateWindow()

    def save_download(self, download):
        suggested = Path(download.downloadDirectory()) / download.suggestedFileName()
        destination, _ = QFileDialog.getSaveFileName(self, "保存文件", str(suggested))
        if destination:
            path = Path(destination)
            download.setDownloadDirectory(str(path.parent))
            download.setDownloadFileName(path.name)
            download.accept()
        else:
            download.cancel()

    def bring_forward(self):
        if self.isMinimized():
            self.showNormal()
        self.show()
        self.raise_()
        self.activateWindow()

    def activate_from_tray(self, reason):
        if reason in (QSystemTrayIcon.ActivationReason.Trigger, QSystemTrayIcon.ActivationReason.DoubleClick):
            self.bring_forward()

    def closeEvent(self, event):
        if QSystemTrayIcon.isSystemTrayAvailable():
            self.hide()
            event.ignore()
        else:
            event.accept()
            QApplication.instance().quit()


def run_desktop_window(server, icon_path, show_request, keep_running):
    if sys.platform == "win32":
        from ctypes import windll

        windll.shell32.SetCurrentProcessExplicitAppUserModelID("AutoTest.Desktop")
    app = QApplication(["Auto-Test"])
    app.setApplicationName("Auto-Test")
    app.setQuitOnLastWindowClosed(False)
    icon = QIcon(str(icon_path))
    app.setWindowIcon(icon)
    window = DesktopWindow(app, icon, QUrl(server))
    timer = QTimer(window)

    def update():
        if not keep_running():
            app.quit()
            return
        if show_request.exists():
            show_request.unlink(missing_ok=True)
            window.bring_forward()

    timer.timeout.connect(update)
    timer.start(500)
    window.show()
    app.exec()
    window.tray.hide()
