"""pytest fixtures：提供 browser session 生命周期"""

import pytest
from pathlib import Path

from auto_ai.browser import browser_session


@pytest.fixture
async def session():
    """每个测试用例共享一个浏览器会话"""
    async with browser_session(headless=True, cache_dir=Path("cache")) as s:
        yield s

    # 关闭 Neo4j 连接
    try:
        from auto_ai.knowledge_graph import get_kg
        get_kg().close()
    except Exception:
        pass
