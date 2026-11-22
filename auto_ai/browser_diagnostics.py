"""Bounded, redacted diagnostics for Playwright page callbacks."""

from dataclasses import dataclass, field
import re
from typing import Any
from urllib.parse import urlsplit, urlunsplit


_SENSITIVE_ASSIGNMENT = re.compile(
    r"(?i)\b(bearer)\s+[^\s,;]+|\b(password|passwd|token|api[_-]?key|secret)\b\s*[:=]\s*(['\"]?)[^\s,;\"']+\3"
)
_URL = re.compile(r"https?://[^\s\"'<>]+", re.IGNORECASE)
_DEFAULT_MAX_EVENTS = 60
_DEFAULT_TEXT_LIMIT = 500


def safe_url(value: Any) -> str:
    """Return an HTTP URL without userinfo, query, or fragment data."""
    try:
        parts = urlsplit(str(value))
        if parts.scheme.lower() not in {"http", "https"} or not parts.netloc:
            return "[invalid URL]"
        return urlunsplit((parts.scheme, parts.netloc.rsplit("@", 1)[-1], parts.path, "", ""))
    except (TypeError, ValueError):
        return "[invalid URL]"


def redact_urls(text: str) -> str:
    return _URL.sub(lambda match: safe_url(match.group(0)), text)


def safe_text(value: Any, limit: int = _DEFAULT_TEXT_LIMIT) -> str:
    """Redact common credential forms, sanitize URLs, and bound text length."""
    text = redact_urls(str(value or ""))

    def redact(match: re.Match[str]) -> str:
        if match.group(1):
            return "Bearer [REDACTED]"
        return f"{match.group(2)}=[REDACTED]"

    text = _SENSITIVE_ASSIGNMENT.sub(redact, text)
    text = re.sub(r"[\r\n\t]+", " ", text)
    return text[:max(0, limit)]


@dataclass(frozen=True)
class DiagnosticEvent:
    kind: str
    data: dict[str, Any] = field(default_factory=dict)


class BrowserDiagnostics:
    """Collect bounded event summaries and bind callbacks to a Playwright page."""

    def __init__(self, max_events: int = _DEFAULT_MAX_EVENTS, text_limit: int = _DEFAULT_TEXT_LIMIT):
        self.max_events = max(0, max_events)
        self.text_limit = max(0, text_limit)
        self.events: list[DiagnosticEvent] = []

    def record(self, kind: str, **data: Any) -> DiagnosticEvent | None:
        """Append one summarized event, dropping entries after the configured cap."""
        if len(self.events) >= self.max_events:
            return None

        safe_data: dict[str, Any] = {}
        for name, value in data.items():
            if name in {"url", "request_url"}:
                safe_data[name] = safe_url(value)
            elif isinstance(value, str):
                safe_data[name] = safe_text(value, self.text_limit)
            elif isinstance(value, (int, float, bool)) or value is None:
                safe_data[name] = value
        event = DiagnosticEvent(kind=kind, data=safe_data)
        self.events.append(event)
        return event

    def bind_page(self, page: Any) -> None:
        """Attach the diagnostic callbacks supported by a Playwright Page."""
        page.on("requestfailed", self._on_request_failed)
        page.on("response", self._on_response)
        page.on("pageerror", self._on_page_error)
        page.on("crash", self._on_crash)

    def _on_request_failed(self, request: Any) -> None:
        self.record("request_failed", method=request.method, url=request.url, text=request.failure)

    def _on_response(self, response: Any) -> None:
        status = response.status
        if status >= 400:
            request = response.request
            self.record(
                "http_error",
                status=status,
                method=request.method,
                url=response.url,
                text=getattr(response, "status_text", ""),
            )

    def _on_page_error(self, error: Any) -> None:
        self.record("page_exception", text=getattr(error, "message", str(error)))

    def _on_crash(self, *_args: Any) -> None:
        self.record("crash", text="页面崩溃")


def classify_events(events: list[DiagnosticEvent]) -> dict[str, list[DiagnosticEvent]]:
    """Group collected evidence into the four supported diagnostic categories."""
    category_by_kind = {
        "request_failed": "request_failure",
        "http_error": "http_error",
        "page_exception": "page_exception",
        "crash": "crash",
    }
    categories = {category: [] for category in category_by_kind.values()}
    for event in events:
        category = category_by_kind.get(event.kind)
        if category:
            categories[category].append(event)
    return categories
