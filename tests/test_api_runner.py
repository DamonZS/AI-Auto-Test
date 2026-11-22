import asyncio
import json
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from agents.api_runner import execute_api_task


class EchoHandler(BaseHTTPRequestHandler):
    requests = []

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        parsed_body = json.loads(body) if body else None
        type(self).requests.append(
            {
                "method": self.command,
                "path": self.path,
                "headers": dict(self.headers.items()),
                "body": parsed_body,
            }
        )
        response = json.dumps(
            {"accepted": {"name": (parsed_body or {}).get("name"), "items": [{"id": 7}]}}
        ).encode()
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(response)))
        self.end_headers()
        self.wfile.write(response)

    def log_message(self, _format, *_args):
        return


class ApiRunnerTests(unittest.IsolatedAsyncioTestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), EchoHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.thread.join()
        cls.server.server_close()

    async def asyncSetUp(self):
        EchoHandler.requests.clear()

    async def test_executes_request_and_all_supported_assertions(self):
        task = {
            "base_url": self.base_url,
            "scenario": {
                "request": {
                    "method": "POST",
                    "path": "/echo",
                    "headers": {"Authorization": "Bearer header-secret"},
                    "query": {"search": "{{term}}"},
                    "json": {"name": "{{name}}", "password": "body-secret"},
                    "timeout_seconds": 2,
                },
                "assertions": [
                    {"type": "status_code", "expected": 201},
                    {"type": "json_equals", "path": "accepted.name", "expected": "Ada"},
                    {"type": "json_exists", "path": "accepted.items[0].id"},
                    {"type": "response_time", "max_ms": 500},
                ],
            },
            "data": {"term": "compiler", "name": "Ada"},
        }

        result = await execute_api_task(task)

        self.assertEqual("passed", result["status"])
        self.assertTrue(result["passed"])
        self.assertFalse(result["needs_review"])
        self.assertEqual([], result["failures"])
        request = EchoHandler.requests[0]
        self.assertEqual("POST", request["method"])
        self.assertIn("search=compiler", request["path"])
        self.assertEqual("Bearer header-secret", request["headers"]["Authorization"])
        self.assertEqual("Ada", request["body"]["name"])
        self.assertEqual("body-secret", request["body"]["password"])

    async def test_failed_assertion_is_reported_without_request_secrets(self):
        task = {
            "base_url": self.base_url,
            "scenario": {
                "request": {
                    "method": "POST",
                    "path": "/echo",
                    "headers": {"Authorization": "Bearer header-secret"},
                    "json": {"name": "Ada", "password": "body-secret"},
                },
                "assertions": [
                    {"type": "json_equals", "path": "accepted.name", "expected": "Grace"}
                ],
            },
            "data": {},
        }

        result = await execute_api_task(task)

        self.assertEqual("failed", result["status"])
        self.assertFalse(result["passed"])
        self.assertFalse(result["needs_review"])
        self.assertEqual(1, len(result["failures"]))
        serialized = json.dumps(result)
        self.assertNotIn("header-secret", serialized)
        self.assertNotIn("body-secret", serialized)
        self.assertNotIn("Grace", serialized)

    async def test_no_assertions_requires_review_after_request(self):
        task = {
            "base_url": self.base_url,
            "scenario": {"request": {"method": "POST", "path": "/echo"}, "assertions": []},
            "data": {},
        }

        result = await execute_api_task(task)

        self.assertEqual("needs_review", result["status"])
        self.assertFalse(result["passed"])
        self.assertTrue(result["needs_review"])
        self.assertEqual(1, len(EchoHandler.requests))

    async def test_request_cannot_leave_base_url_origin(self):
        task = {
            "base_url": self.base_url,
            "scenario": {
                "request": {"method": "GET", "path": "https://example.invalid/private"},
                "assertions": [{"type": "status_code", "expected": 200}],
            },
            "data": {},
        }

        result = await execute_api_task(task)

        self.assertEqual("error", result["status"])
        self.assertFalse(result["passed"])
        self.assertEqual([], EchoHandler.requests)
        self.assertNotIn("example.invalid", json.dumps(result))

    async def test_response_time_assertion_uses_measured_duration(self):
        started = time.monotonic()
        result = await execute_api_task(
            {
                "base_url": self.base_url,
                "scenario": {
                    "request": {"method": "POST", "path": "/echo"},
                    "assertions": [{"type": "response_time", "max_ms": 0}],
                },
                "data": {},
            }
        )

        self.assertEqual("failed", result["status"])
        self.assertGreaterEqual(result["duration_ms"], 0)
        self.assertGreaterEqual(time.monotonic() - started, 0)


if __name__ == "__main__":
    unittest.main()
