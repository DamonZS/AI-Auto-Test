"""配置模块"""

import os
from dotenv import load_dotenv

load_dotenv()


class Config:
    # LLM
    OPENAI_API_KEY = os.getenv("OPENAI_API_KEY", "")
    OPENAI_BASE_URL = os.getenv("OPENAI_BASE_URL", "https://api.openai.com/v1")
    PLANNER_MODEL = os.getenv("PLANNER_MODEL", "gpt-4o")
    EXECUTOR_MODEL = os.getenv("EXECUTOR_MODEL", "gpt-4o-mini")
    TEMPERATURE = 0

    # Neo4j 知识图谱
    NEO4J_ENABLED = os.getenv("NEO4J_ENABLED", "true").lower() in ("true", "1", "yes")
    NEO4J_URI = os.getenv("NEO4J_URI", "bolt://localhost:7687")
    NEO4J_USER = os.getenv("NEO4J_USER", "neo4j")
    NEO4J_PASSWORD = os.getenv("NEO4J_PASSWORD", "")
    NEO4J_PROJECT = os.getenv("NEO4J_PROJECT", "auto_ai")   # 项目名，作为节点标签隔离数据


config = Config()
