import asyncio
import json
import re

import pytest

from agents.perf_runner import execute_perf_task


async def _start_server(status=200, delay=0):
    requests = 0

    async def handle(reader, writer):
        nonlocal requests
        requests += 1
        await reader.readuntil(b"\r\n\r\n")
        if delay:
            await asyncio.sleep(delay)
        body = b'{"secret":"do-not-report"}'
        writer.write(
            f"HTTP/1.1 {status} Test\r\nContent-Length: {len(body)}\r\n"
            "Content-Type: application/json\r\nConnection: close\r\n\r\n".encode()
            + body
        )
        await writer.drain()
        writer.close()
        await writer.wait_closed()

    server = await asyncio.start_server(handle, "127.0.0.1", 0)
    return server, server.sockets[0].getsockname()[1], lambda: requests


def test_perf_runner_uses_real_http_and_collects_metrics():
    async def run():
        server, port, request_count = await _start_server()
        try:
            return await execute_perf_task(
                {
                    "base_url": f"http://127.0.0.1:{port}",
                    "scenario": {"method": "GET", "path": "/health"},
                    "concurrency": 2,
                    "max_requests": 5,
                    "duration_seconds": 10,
                    "timeout": 1,
                    "headers": {"Authorization": "Bearer secret"},
                }
            ), request_count()
        finally:
            server.close()
            await server.wait_closed()

    result, actual_requests = asyncio.run(run())

    assert result["status"] == "passed"
    assert result["requests"] == 5
    assert result["failures"] == 0
    assert result["error_rate"] == 0
    assert result["rps"] > 0
    assert result["p50_ms"] <= result["p95_ms"] <= result["p99_ms"]
    assert actual_requests == 5
    assert "secret" not in json.dumps(result)


def test_perf_runner_counts_http_failures_without_leaking_payloads():
    async def run():
        server, port, _ = await _start_server(status=503)
        try:
            return await execute_perf_task(
            {
                "base_url": f"http://127.0.0.1:{port}",
                "scenario": json.dumps(
                    {
                        "method": "POST",
                        "path": "/submit",
                        "headers": {"X-Token": "top-secret"},
                        "query": {"q": "private"},
                        "json": {"password": "hidden"},
                    }
                ),
                "concurrency": 1,
                "max_requests": 2,
                "duration_seconds": 10,
                "timeout": 1,
            }
            )
        finally:
            server.close()
            await server.wait_closed()

    result = asyncio.run(run())

    assert result["status"] == "failed"
    assert result["requests"] == 2
    assert result["failures"] == 2
    assert result["error_rate"] == 1
    assert not re.search(r"top-secret|private|hidden|do-not-report", json.dumps(result))


def test_perf_runner_returns_needs_review_for_missing_or_invalid_limits():
    cases = [
        {"base_url": "http://127.0.0.1:1", "scenario": {"method": "GET"}},
        {
            "base_url": "http://127.0.0.1:1",
            "scenario": {"method": "GET", "path": "/"},
            "concurrency": 0,
        },
        {
            "base_url": "http://127.0.0.1:1",
            "scenario": {"method": "GET", "path": "/"},
            "duration_seconds": 301,
        },
    ]

    for task in cases:
        result = asyncio.run(execute_perf_task(task))
        assert result["status"] == "needs_review"
        assert result["needs_review"] is True
        assert result["requests"] == 0


def test_perf_runner_reports_timeout_as_failure():
    async def run():
        server, port, _ = await _start_server(delay=0.05)
        try:
            return await execute_perf_task(
            {
                "base_url": f"http://127.0.0.1:{port}",
                "scenario": {"method": "GET", "path": "/slow"},
                "concurrency": 1,
                "max_requests": 1,
                "duration_seconds": 1,
                "timeout": 0.001,
            }
            )
        finally:
            server.close()
            await server.wait_closed()

    result = asyncio.run(run())

    assert result["status"] == "failed"
    assert result["requests"] == 1
    assert result["failures"] == 1
    assert result["error_rate"] == 1
    assert result["errors"][0]["type"] in {"ReadTimeout", "ConnectTimeout"}
