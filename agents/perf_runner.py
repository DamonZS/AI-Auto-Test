"""受控异步 HTTP 性能测试执行器。"""

import asyncio
import json
import math
import time
from urllib.parse import urljoin, urlsplit

import httpx


LIMITS = {
    "concurrency": (1, 100),
    "duration_seconds": (0.1, 300),
    "max_requests": (1, 10000),
    "timeout": (0.001, 60),
}


def _review(reason):
    return {
        "status": "needs_review",
        "passed": False,
        "needs_review": True,
        "requests": 0,
        "failures": 0,
        "error_rate": 0,
        "rps": 0,
        "p50_ms": None,
        "p95_ms": None,
        "p99_ms": None,
        "errors": [],
        "report": f"性能测试参数需要人工复核：{reason}",
    }


def _number(task, scenario, name, default):
    value = task.get(name)
    if value is None:
        value = scenario.get(name, default)
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{name} 必须是数字")
    low, high = LIMITS[name]
    if not low <= value <= high:
        raise ValueError(f"{name} 超出允许范围")
    return value


def _scenario(task):
    scenario = task.get("scenario")
    if isinstance(scenario, str):
        scenario = json.loads(scenario)
    if not isinstance(scenario, dict):
        raise ValueError("scenario 必须是对象或 JSON 字符串")
    request = scenario.get("request", scenario)
    if not isinstance(request, dict):
        raise ValueError("scenario.request 必须是对象")
    method = request.get("method")
    path = request.get("path")
    if not isinstance(method, str) or not method.strip() or not isinstance(path, str) or not path.strip():
        raise ValueError("场景必须包含可执行的 method 和 path")
    method = method.upper()
    if method not in {"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}:
        raise ValueError("method 不受支持")
    return {
        "method": method,
        "path": path,
        "headers": request.get("headers", task.get("headers")) or {},
        "query": request.get("query", task.get("query")) or {},
        "json": request.get("json", task.get("json")),
        "concurrency": task.get("concurrency", scenario.get("concurrency", 1)),
        "duration_seconds": task.get("duration_seconds", scenario.get("duration_seconds", 10)),
        "max_requests": task.get("max_requests", scenario.get("max_requests", 100)),
        "timeout": task.get("timeout", scenario.get("timeout", 10)),
    }


def _url(base_url, path):
    if not isinstance(base_url, str):
        raise ValueError("base_url 必须是字符串")
    base = urlsplit(base_url)
    if base.scheme not in {"http", "https"} or not base.netloc:
        raise ValueError("base_url 必须是有效的 HTTP(S) 地址")
    target = urlsplit(urljoin(base_url.rstrip("/") + "/", path))
    if (target.scheme.lower(), target.netloc.lower()) != (base.scheme.lower(), base.netloc.lower()):
        raise ValueError("path 不得超出 base_url origin")
    return target.geturl()


def _percentile(values, percentile):
    if not values:
        return None
    values = sorted(values)
    index = min(len(values) - 1, math.ceil(percentile * len(values)) - 1)
    return round(values[index], 3)


async def execute_perf_task(task: dict, settings: dict | None = None) -> dict:
    try:
        scenario = _scenario(task)
        base_url = task.get("base_url")
        url = _url(base_url, scenario["path"])
        concurrency = int(_number({}, scenario, "concurrency", 1))
        duration = float(_number({}, scenario, "duration_seconds", 10))
        max_requests = int(_number({}, scenario, "max_requests", 100))
        timeout = float(_number({}, scenario, "timeout", 10))
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as exc:
        return _review(type(exc).__name__)

    started = time.monotonic()
    deadline = started + duration
    latencies = []
    errors = []
    outcomes = []
    next_request = 0
    lock = asyncio.Lock()

    async def request_loop(client):
        nonlocal next_request
        while True:
            async with lock:
                if next_request >= max_requests or (next_request and time.monotonic() >= deadline):
                    return
                next_request += 1
            request_started = time.monotonic()
            try:
                response = await client.request(
                    scenario["method"],
                    url,
                    headers=scenario["headers"],
                    params=scenario["query"],
                    json=scenario["json"],
                )
                elapsed = (time.monotonic() - request_started) * 1000
                latencies.append(elapsed)
                if response.status_code >= 400:
                    raise httpx.HTTPStatusError(
                        "HTTP error", request=response.request, response=response
                    )
                outcomes.append(True)
            except Exception as exc:
                outcomes.append(False)
                if len(errors) < 10:
                    errors.append({"type": type(exc).__name__})

    async with httpx.AsyncClient(timeout=timeout) as client:
        await asyncio.gather(*(request_loop(client) for _ in range(concurrency)))

    elapsed_seconds = max(time.monotonic() - started, 0.001)
    completed = len(outcomes)
    failures = outcomes.count(False)
    error_rate = failures / completed if completed else 0
    return {
        "status": "passed" if completed and failures == 0 else "failed",
        "passed": completed > 0 and failures == 0,
        "needs_review": False,
        "requests": completed,
        "failures": failures,
        "error_rate": round(error_rate, 4),
        "rps": round(completed / elapsed_seconds, 3),
        "p50_ms": _percentile(latencies, 0.50),
        "p95_ms": _percentile(latencies, 0.95),
        "p99_ms": _percentile(latencies, 0.99),
        "errors": errors,
        "report": f"完成 {completed} 个请求，失败 {failures} 个。",
    }
