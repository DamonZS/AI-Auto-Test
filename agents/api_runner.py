"""Deterministic HTTP scenario runner for heartbeat API tasks."""

import json
import re
import time
from urllib.parse import parse_qsl, urlencode, urljoin, urlsplit, urlunsplit

import httpx


_DATA_TOKEN = re.compile(r"\{\{([A-Za-z0-9_.-]+)\}\}")


def _substitute(value, data):
    if isinstance(value, str):
        return _DATA_TOKEN.sub(lambda match: str(data[match.group(1)]), value)
    if isinstance(value, list):
        return [_substitute(item, data) for item in value]
    if isinstance(value, dict):
        return {key: _substitute(item, data) for key, item in value.items()}
    return value


def _request_url(base_url, path, query, data):
    base = urlsplit(base_url)
    if base.scheme not in {"http", "https"} or not base.hostname:
        raise ValueError("base_url 必须是有效的 HTTP(S) 地址")
    resolved = urlsplit(urljoin(base_url.rstrip("/") + "/", str(path)))
    base_port = base.port or (443 if base.scheme.lower() == "https" else 80)
    resolved_port = resolved.port or (443 if resolved.scheme.lower() == "https" else 80)
    if (resolved.scheme.lower(), resolved.hostname.lower(), resolved_port) != (
        base.scheme.lower(), base.hostname.lower(), base_port
    ):
        raise ValueError("请求地址超出项目 base_url origin")

    query_values = parse_qsl(resolved.query, keep_blank_values=True)
    if query:
        query_values.extend((key, str(value)) for key, value in _substitute(query, data).items())
    return urlunsplit(
        (resolved.scheme, resolved.netloc, resolved.path, urlencode(query_values), "")
    )


def _json_value(document, path):
    current = document
    for part in re.split(r"\.(?![^[]*\])", path):
        match = re.fullmatch(r"([^[]*)(?:\[(\d+)\])?", part)
        if not match:
            raise KeyError(path)
        key, index = match.groups()
        if key:
            if not isinstance(current, dict) or key not in current:
                raise KeyError(path)
            current = current[key]
        if index is not None:
            if not isinstance(current, list) or int(index) >= len(current):
                raise KeyError(path)
            current = current[int(index)]
    return current


def _check_assertion(assertion, status_code, payload, duration_ms):
    kind = assertion["type"]
    if kind == "status_code":
        actual = status_code
        expected = assertion["expected"]
        passed = actual == expected
    elif kind in {"json_equals", "json_exists"}:
        path = assertion["path"]
        try:
            actual = _json_value(payload, path)
            exists = True
        except (KeyError, TypeError):
            actual = None
            exists = False
        if kind == "json_exists":
            passed = exists
            expected = "exists"
        else:
            expected = assertion["expected"]
            passed = exists and actual == expected
        actual = "present" if exists else "missing"
    elif kind == "response_time":
        actual = duration_ms
        expected = assertion["max_ms"]
        passed = actual <= expected
    else:
        raise ValueError("断言类型无效")

    return {
        "type": kind,
        "passed": passed,
        "path": assertion.get("path", ""),
        "expected": expected if kind in {"status_code", "response_time"} else None,
        "actual": actual if kind in {"status_code", "response_time"} else None,
    }


async def execute_api_task(task: dict, settings: dict | None = None) -> dict:
    started = time.monotonic()
    failures = []
    report = []
    status = "error"

    try:
        scenario = task["scenario"]
        if isinstance(scenario, str):
            scenario = json.loads(scenario)
        request = scenario["request"]
        assertions = scenario.get("assertions", [])
        data = task.get("data") or {}
        url = _request_url(task["base_url"], request["path"], request.get("query"), data)
        body = _substitute(request.get("json"), data)
        headers = _substitute(request.get("headers", {}), data)
        timeout = request.get("timeout_seconds", 30)

        async with httpx.AsyncClient(timeout=timeout, trust_env=False) as client:
            request_started = time.monotonic()
            response = await client.request(
                request.get("method", "GET"),
                url,
                headers=headers,
                params=None,
                json=body,
            )
        response_duration_ms = int((time.monotonic() - request_started) * 1000)
        duration_ms = int((time.monotonic() - started) * 1000)
        try:
            payload = response.json()
        except (ValueError, json.JSONDecodeError):
            payload = None

        if not assertions:
            status = "needs_review"
            report.append("请求已执行，但场景没有断言，结果需要人工复核。")
        else:
            for index, assertion in enumerate(assertions, 1):
                result = _check_assertion(
                    assertion, response.status_code, payload, response_duration_ms
                )
                report.append(
                    f"断言 {index} ({result['type']}): {'通过' if result['passed'] else '失败'}"
                )
                if not result["passed"]:
                    failures.append(
                        {
                            "signature": f"api_assertion:{index}:{result['type']}:{result['path']}",
                            "title": f"接口断言失败：{result['type']}",
                            "evidence": (
                                f"path={result['path']} expected={result['expected']} "
                                f"actual={result['actual']}"
                            ),
                        }
                    )
            status = "failed" if failures else "passed"
    except Exception as exc:
        duration_ms = int((time.monotonic() - started) * 1000)
        status = "error"
        report.append(f"接口测试执行异常：{type(exc).__name__}")
        failures.append(
            {
                "signature": f"api_execution:{type(exc).__name__}",
                "title": "接口测试执行异常",
                "evidence": type(exc).__name__,
            }
        )

    return {
        "status": status,
        "passed": status == "passed",
        "needs_review": status == "needs_review",
        "duration_ms": duration_ms,
        "report": "\n".join(report),
        "failures": failures,
        "token_in": 0,
        "token_out": 0,
    }
