"""auto_ai - LangGraph + Playwright 自动化测试框架"""

from importlib import import_module

from auto_ai.config import config

__all__ = ["config", "run_test", "BrowserSession", "browser_session"]


def __getattr__(name):
    if name == "run_test":
        return import_module("auto_ai.agent").run_test
    if name in {"BrowserSession", "browser_session"}:
        return getattr(import_module("auto_ai.browser"), name)
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
