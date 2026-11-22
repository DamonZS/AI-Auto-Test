"""Run the platform worker with a scoped service token."""

import argparse
import asyncio
import os
from pathlib import Path

from dotenv import dotenv_values


TOKEN_NAME = "PLATFORM_WORKER_TOKEN"
ROOT = Path(__file__).resolve().parent.parent


def load_worker_token() -> str:
    token = os.environ.get(TOKEN_NAME)
    if token is None:
        token = dotenv_values(ROOT / ".env", interpolate=False).get(TOKEN_NAME)
    return (token or "").strip()


async def run(server: str, poll: float, token: str) -> None:
    import httpx

    from agents.ui_runner import execute_task, request

    headers = {"Authorization": f"Bearer {token}"}
    heartbeat = f"{server}/api/runs/heartbeat"
    if os.environ.get("PLATFORM_KNOWLEDGE_DIR"):
        heartbeat += "?knowledge=requirements"
    async with httpx.AsyncClient(timeout=30) as client:
        while True:
            task_response = await request(
                client, "POST", heartbeat, headers, json={}
            )
            task = task_response.get("run")
            if not task:
                await asyncio.sleep(poll)
                continue
            settings = await request(client, "GET", f"{server}/api/agent/settings", headers)
            result = await execute_task(task, settings)
            if task.get("type") == "perf":
                result.pop("failures", None)
            await request(
                client,
                "POST",
                f"{server}/api/runs/{task['run_id']}/finish",
                headers,
                json=result,
            )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--server", required=True)
    parser.add_argument("--poll", type=float, default=2)
    args = parser.parse_args()
    token = load_worker_token()
    if not token:
        parser.error(f"{TOKEN_NAME} is required")
    asyncio.run(run(args.server.rstrip("/"), args.poll, token))


if __name__ == "__main__":
    main()
