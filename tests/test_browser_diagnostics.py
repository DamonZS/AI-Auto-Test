import unittest
import importlib.util
from pathlib import Path

MODULE_PATH = Path(__file__).resolve().parents[1] / "auto_ai" / "browser_diagnostics.py"
SPEC = importlib.util.spec_from_file_location("browser_diagnostics", MODULE_PATH)
browser_diagnostics = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(browser_diagnostics)
BrowserDiagnostics = browser_diagnostics.BrowserDiagnostics
classify_events = browser_diagnostics.classify_events
safe_text = browser_diagnostics.safe_text
safe_url = browser_diagnostics.safe_url


class Request:
    def __init__(self, url, method="GET", failure_text="net::ERR_FAILED"):
        self.url = url
        self.method = method
        self.failure = failure_text


class Response:
    def __init__(self, url, status=500, method="POST"):
        self.url = url
        self.status = status
        self.request = Request(url, method)
        self.status_text = "Server Error"


class PageError:
    message = "token=private-value"


class FakePage:
    def __init__(self):
        self.handlers = {}

    def on(self, event, callback):
        self.handlers[event] = callback


class BrowserDiagnosticsTests(unittest.TestCase):
    def test_safe_url_removes_userinfo_query_and_fragment(self):
        self.assertEqual(
            "https://example.test/path",
            safe_url("https://fixture-user@example.test/path?token=secret#private"),
        )

    def test_safe_text_redacts_sensitive_values_and_limits_length(self):
        value = "Bearer abc token=token-value password=hunter2 api_key: key-value secret='hidden' " + "x" * 100
        result = safe_text(value, limit=80)
        self.assertNotIn("abc", result)
        self.assertNotIn("token-value", result)
        self.assertNotIn("hunter2", result)
        self.assertNotIn("key-value", result)
        self.assertNotIn("hidden", result)
        self.assertLessEqual(len(result), 80)

    def test_callbacks_record_only_safe_event_data(self):
        diagnostics = BrowserDiagnostics(max_events=10)
        page = FakePage()
        diagnostics.bind_page(page)

        page.handlers["requestfailed"](
            Request("https://example.test/api?password=secret", failure_text="Bearer leaked")
        )
        page.handlers["response"](Response("https://example.test/api?token=secret", 503))
        page.handlers["pageerror"](PageError())
        page.handlers["crash"]()

        self.assertEqual(["request_failed", "http_error", "page_exception", "crash"], [e.kind for e in diagnostics.events])
        serialized = repr(diagnostics.events)
        self.assertNotIn("secret", serialized)
        self.assertNotIn("leaked", serialized)
        self.assertNotIn("private-value", serialized)
        self.assertTrue(all("headers" not in event.data and "body" not in event.data for event in diagnostics.events))

    def test_classify_events_by_evidence(self):
        diagnostics = BrowserDiagnostics()
        diagnostics.record("request_failed", url="https://example.test/api", text="timeout")
        diagnostics.record("http_error", url="https://example.test/api", status=500)
        diagnostics.record("page_exception", text="render failed")
        diagnostics.record("crash", text="page crashed")

        classified = classify_events(diagnostics.events)
        self.assertEqual(1, len(classified["request_failure"]))
        self.assertEqual(1, len(classified["http_error"]))
        self.assertEqual(1, len(classified["page_exception"]))
        self.assertEqual(1, len(classified["crash"]))

    def test_event_limit_keeps_the_first_events(self):
        diagnostics = BrowserDiagnostics(max_events=2)
        diagnostics.record("page_exception", text="first")
        diagnostics.record("page_exception", text="second")
        diagnostics.record("crash", text="third")

        self.assertEqual(2, len(diagnostics.events))
        self.assertEqual(["first", "second"], [event.data["text"] for event in diagnostics.events])


if __name__ == "__main__":
    unittest.main()
