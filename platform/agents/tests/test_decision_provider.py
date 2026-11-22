import importlib.util
import json
import unittest
from pathlib import Path
from unittest.mock import patch

import httpx

MODULE = Path(__file__).resolve().parents[1] / "decision_provider.py"
spec = importlib.util.spec_from_file_location("decision_provider", MODULE)
decision_provider = importlib.util.module_from_spec(spec)
spec.loader.exec_module(decision_provider)
DecisionProviderError = decision_provider.DecisionProviderError
LayaDecisionProvider = decision_provider.LayaDecisionProvider

CRITERIA = {"搜索": "点击搜索", "停止": "不执行"}


def response(answer=None, status_code=200):
    answer = answer or {"choice": "搜索", "probabilities": {"搜索": 0.9, "停止": 0.1}, "confidence": 0.8}
    return httpx.Response(
        status_code,
        json={"choices": [{"message": {"content": json.dumps({"latency_ms": 12, "result": {"answers": {"scenario": answer}}})}}]},
        request=httpx.Request("POST", "https://gateway.test/v1/chat/completions"),
    )


class DecisionProviderTests(unittest.IsolatedAsyncioTestCase):
    def provider(self):
        return LayaDecisionProvider("https://gateway.test/v1", "decision-model", "secret")

    async def test_sends_state_and_candidate_contract_and_returns_scores(self):
        seen = {}

        async def post(_client, url, **kwargs):
            seen.update(url=url, **kwargs)
            return response()

        with patch("httpx.AsyncClient.post", new=post):
            result = await self.provider().choose("点击搜索", CRITERIA, {"phase": "target"})

        payload = json.loads(seen["json"]["messages"][0]["content"])
        self.assertEqual(seen["url"], "https://gateway.test/v1/chat/completions")
        self.assertEqual(payload["state"], "点击搜索")
        self.assertEqual(payload["questions"]["scenario"]["criteria"], CRITERIA)
        self.assertEqual(result["choice"], "搜索")
        self.assertEqual(result["margin"], 0.8)
        self.assertEqual(result["meta"], {"phase": "target"})
        self.assertNotIn("secret", json.dumps(seen["json"]))

    def test_rejects_invalid_choice_or_probability_distribution(self):
        answers = [
            {"choice": "删除", "probabilities": {"搜索": 0.9, "停止": 0.1}},
            {"choice": "搜索", "probabilities": {"搜索": 0.9}},
            {"choice": "搜索", "probabilities": {"搜索": 0.1, "停止": 0.9}},
            {"choice": "搜索", "probabilities": {"搜索": float("nan"), "停止": 0.1}},
        ]
        for answer in answers:
            payload = {"choices": [{"message": {"content": json.dumps({"result": {"answers": {"scenario": answer}}})}}]}
            with self.subTest(answer=answer), self.assertRaises(DecisionProviderError):
                LayaDecisionProvider._decode(payload, CRITERIA)

    async def test_rejects_http_failure_without_echoing_response_or_secret(self):
        async def post(_client, _url, **_kwargs):
            raise httpx.RequestError("secret", request=httpx.Request("POST", "https://gateway.test"))
        with patch("httpx.AsyncClient.post", new=post):
            with self.assertRaises(DecisionProviderError) as raised:
                await self.provider().choose("状态", CRITERIA)
        self.assertNotIn("secret", str(raised.exception))

    def test_requires_https_and_credentials(self):
        with self.assertRaises(ValueError):
            LayaDecisionProvider("http://gateway.test/v1", "model", "key")
        with self.assertRaises(ValueError):
            LayaDecisionProvider("https://gateway.test/v1", "model", " ")


if __name__ == "__main__":
    unittest.main()





