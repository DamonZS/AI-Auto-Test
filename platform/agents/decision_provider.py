"""OpenAI-compatible decision provider with an explicit candidate-probability contract."""

import json
import math
import time
from collections.abc import Mapping

import httpx


class DecisionProviderError(RuntimeError):
    """The model response cannot safely drive a UI decision."""


class LayaDecisionProvider:
    def __init__(self, base_url: str, model: str, api_key: str, timeout: float = 30.0):
        base = httpx.URL(base_url)
        if base.scheme != "https" or base.username or base.password or base.query or base.fragment:
            raise ValueError("决策 API 地址必须为不含凭据或查询参数的 HTTPS 地址")
        if not model or not api_key.strip():
            raise ValueError("决策模型名称和 API key 必填")
        self.base_url = str(base).rstrip("/")
        self.model = model
        self.api_key = api_key
        self.timeout = timeout

    async def choose(self, state: str, criteria: Mapping[str, str], meta: Mapping | None = None) -> dict:
        if len(criteria) < 2:
            raise ValueError("决策至少需要两个候选项")
        started = time.monotonic()
        body = {
            "model": self.model,
            "stream": False,
            "messages": [{
                "role": "user",
                "content": json.dumps({
                    "state": state,
                    "questions": {"scenario": {
                        "type": "choice",
                        "instructions": "用户请求对应哪个操作？",
                        "criteria": criteria,
                    }},
                }, ensure_ascii=False),
            }],
        }
        try:
            async with httpx.AsyncClient(timeout=self.timeout, follow_redirects=False) as client:
                response = await client.post(
                    f"{self.base_url}/chat/completions",
                    headers={"Authorization": f"Bearer {self.api_key}"},
                    json=body,
                )
            if response.is_redirect:
                raise DecisionProviderError("决策 API 拒绝重定向")
            response.raise_for_status()
            envelope = response.json()
        except DecisionProviderError:
            raise
        except (httpx.HTTPError, ValueError):
            raise DecisionProviderError("决策 API 请求失败或响应无效") from None

        result = self._decode(envelope, criteria)
        result["provider"] = "laya-api"
        result["model"] = self.model
        result["inference_ms"] = round((time.monotonic() - started) * 1000, 2)
        if meta:
            result["meta"] = dict(meta)
        return result

    @staticmethod
    def _decode(envelope: dict, criteria: Mapping[str, str]) -> dict:
        try:
            content = envelope["choices"][0]["message"]["content"]
            payload = json.loads(content)
            result = payload["result"]["answers"]["scenario"]
        except (KeyError, IndexError, TypeError, ValueError):
            raise DecisionProviderError("决策 API 未返回可解析的候选结果") from None

        choice = result.get("choice")
        probabilities = result.get("probabilities")
        keys = set(criteria)
        if not isinstance(choice, str) or choice not in keys or not isinstance(probabilities, dict) or set(probabilities) != keys:
            raise DecisionProviderError("决策结果或概率必须完整匹配本次候选集合")
        scores = probabilities.values()
        if any(not isinstance(value, (int, float)) or isinstance(value, bool) or not math.isfinite(value) or not 0 <= value <= 1 for value in scores):
            raise DecisionProviderError("候选概率无效，不使用猜测分数")
        if abs(sum(probabilities.values()) - 1) > 0.02 or any(value > probabilities[choice] for value in probabilities.values()):
            raise DecisionProviderError("概率分布与模型选择不一致")
        ordered = sorted(probabilities.values(), reverse=True)
        confidence = result.get("confidence")
        return {
            "choice": choice,
            "probabilities": probabilities,
            "confidence": confidence if isinstance(confidence, (int, float)) and math.isfinite(confidence) else None,
            "margin": ordered[0] - ordered[1],
            "server_ms": payload.get("latency_ms"),
        }
