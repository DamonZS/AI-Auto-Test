"""Neo4j 知识图谱：记录页面拓扑、区域、元素、API 调用"""
import json
import logging
from pathlib import Path
from typing import Optional, List
from urllib.parse import urlparse

from neo4j import GraphDatabase, Driver

from .config import config

logger = logging.getLogger(__name__)

# ================================================================
# Neo4j Cypher 模板
# ================================================================

# 合并页面节点（url_pattern + project 唯一）
MERGE_PAGE = """
MERGE (p:Page {url_pattern: $url_pattern, project: $project})
SET p.url = $url,
    p.title = coalesce(p.title, $title),
    p.total_elements = $total_elements,
    p.updated_at = datetime()
RETURN p
"""

# 合并区域节点（page + section_name + project 唯一）
MERGE_SECTION = """
MATCH (p:Page {url_pattern: $url_pattern, project: $project})
MERGE (s:Section {name: $section_name, project: $project})-[:PART_OF]->(p)
SET s.tag = $section_tag,
    s.element_count = $element_count,
    s.updated_at = datetime()
RETURN s
"""

# 合并元素节点（section_name 可选）
MERGE_ELEMENT = """
MATCH (p:Page {url_pattern: $url_pattern, project: $project})
MERGE (e:Element {role: $role, name: $name, project: $project})-[:BELONGS_TO]->(p)
SET e.locator = $locator,
    e.tag = $tag,
    e.explored = coalesce(e.explored, false),
    e.updated_at = datetime()
RETURN e
"""

# 将元素关联到区域
LINK_ELEMENT_TO_SECTION = """
MATCH (p:Page {url_pattern: $url_pattern, project: $project})
MATCH (e:Element {role: $role, name: $name, project: $project})-[:BELONGS_TO]->(p)
MATCH (s:Section {name: $section_name, project: $project})-[:PART_OF]->(p)
MERGE (e)-[:IN_SECTION]->(s)
"""

# 标记元素已被探索（被操作过）
MARK_ELEMENT_EXPLORED = """
MATCH (p:Page {url_pattern: $url_pattern, project: $project})
MATCH (e:Element {role: $role, name: $name, project: $project})-[:BELONGS_TO]->(p)
SET e.explored = true, e.updated_at = datetime()
RETURN e
"""

# 记录导航边（页面A → 通过元素X → 页面B）
RECORD_NAVIGATION = """
MATCH (from:Page {url_pattern: $from_pattern, project: $project})
MATCH (to:Page {url_pattern: $to_pattern, project: $project})
MERGE (from)-[r:NAVIGATES_TO {via_role: $role, via_name: $name}]->(to)
SET r.count = coalesce(r.count, 0) + 1,
    r.last_seen = datetime()
RETURN r
"""

# 记录 API 边
RECORD_API = """
MATCH (p:Page {url_pattern: $url_pattern, project: $project})
MATCH (e:Element {role: $role, name: $name, project: $project})-[:BELONGS_TO]->(p)
MERGE (api:API {url: $api_url, method: $method, project: $project})
MERGE (e)-[r:TRIGGERS_API]->(api)
SET r.count = coalesce(r.count, 0) + 1,
    r.last_seen = datetime()
RETURN api
"""

# 查询：页面 + 区域 + 元素 + 导航 → 完整摘要
QUERY_GRAPH_SUMMARY = """
MATCH (p:Page {project: $project})
OPTIONAL MATCH (s:Section {project: $project})-[:PART_OF]->(p)
WITH p, s
ORDER BY s.name
WITH p, collect(DISTINCT {
    name: s.name,
    tag: s.tag,
    element_count: s.element_count
}) AS sections_list
OPTIONAL MATCH (p)-[:BELONGS_TO]-(e:Element {project: $project})
WHERE NOT (e)-[:IN_SECTION]->()
WITH p, sections_list, count(e) AS ungrouped_count
RETURN {
    url_pattern: p.url_pattern,
    title: p.title,
    total_elements: coalesce(p.total_elements, 0),
    sections: sections_list,
    ungrouped: ungrouped_count
} AS page_info
ORDER BY p.url_pattern
"""

# 查询：导航路径
QUERY_NAVIGATION_PATHS = """
MATCH (from:Page {project: $project})-[r:NAVIGATES_TO]->(to:Page {project: $project})
RETURN from.url_pattern AS from_page,
       to.url_pattern AS to_page,
       r.via_role AS via_role,
       r.via_name AS via_name,
       r.count AS traverse_count
ORDER BY from_page, r.count DESC
"""


class KnowledgeGraph:
    """Neo4j 知识图谱封装"""

    def __init__(self, uri: str, user: str, password: str):
        self._driver: Optional[Driver] = None
        self._uri = uri
        self._user = user
        self._password = password

    # ---------- 生命周期 ----------

    def connect(self):
        if not config.NEO4J_ENABLED:
            logger.info("Neo4j 已禁用（NEO4J_ENABLED=false），知识图谱功能跳过")
            return
        try:
            self._driver = GraphDatabase.driver(self._uri, auth=(self._user, self._password))
            self._driver.verify_connectivity()
            self._init_project()
            logger.info("Neo4j 连接成功: %s (project=%s)", self._uri, config.NEO4J_PROJECT)
        except Exception as e:
            logger.warning("Neo4j 连接失败，知识图谱功能将跳过: %s", e)
            self._driver = None

    def _init_project(self):
        """创建项目命名空间索引"""
        try:
            with self._driver.session(database="neo4j") as s:
                for label in ("Page", "Element", "Section", "API"):
                    s.run(f"CREATE INDEX IF NOT EXISTS FOR (n:{label}) ON (n.project)")
        except Exception:
            pass

    def close(self):
        if self._driver:
            self._driver.close()

    def _available(self) -> bool:
        return self._driver is not None

    # ---------- 写入 ----------

    def upsert_page(self, full_url: str, title: str = "", element_count: int = 0):
        """合并页面节点"""
        if not self._available():
            return
        parsed = urlparse(full_url)
        url_pattern = parsed.path or "/"
        try:
            self._driver.execute_query(
                MERGE_PAGE,
                url_pattern=url_pattern,
                url=full_url,
                title=title,
                total_elements=element_count,
                project=config.NEO4J_PROJECT,
                database_="neo4j",
            )
        except Exception as e:
            logger.debug("写入页面节点失败: %s", e)

    def upsert_section(self, full_url: str, section_name: str, section_tag: str,
                       element_count: int = 0):
        """合并区域节点（页面下的逻辑分区）"""
        if not self._available():
            return
        parsed = urlparse(full_url)
        url_pattern = parsed.path or "/"
        try:
            self._driver.execute_query(
                MERGE_SECTION,
                url_pattern=url_pattern,
                section_name=section_name,
                section_tag=section_tag,
                element_count=element_count,
                project=config.NEO4J_PROJECT,
                database_="neo4j",
            )
        except Exception as e:
            logger.debug("写入区域节点失败: %s", e)

    def upsert_element(self, full_url: str, role: str, name: str,
                       locator: str = "", tag: str = "", section_name: str = ""):
        """合并元素节点，可选归属到区域"""
        if not self._available():
            return
        parsed = urlparse(full_url)
        url_pattern = parsed.path or "/"
        try:
            self._driver.execute_query(
                MERGE_ELEMENT,
                url_pattern=url_pattern,
                role=role,
                name=name,
                locator=locator,
                tag=tag,
                project=config.NEO4J_PROJECT,
                database_="neo4j",
            )
            # 有区域 → 链接到区域
            if section_name:
                self._driver.execute_query(
                    LINK_ELEMENT_TO_SECTION,
                    url_pattern=url_pattern,
                    role=role,
                    name=name,
                    section_name=section_name,
                    project=config.NEO4J_PROJECT,
                    database_="neo4j",
                )
        except Exception as e:
            logger.debug("写入元素节点失败: %s", e)

    def set_page_section_counts(self, full_url: str, sections: list):
        """更新页面的区域元素计数（页面级别覆盖率）"""
        if not self._available():
            return
        for sec in sections:
            self.upsert_section(
                full_url,
                sec.get("section", ""),
                sec.get("section_tag", ""),
                len(sec.get("elements", [])),
            )

    def mark_explored(self, full_url: str, role: str, name: str):
        """标记元素已被操作过"""
        if not self._available():
            return
        parsed = urlparse(full_url)
        url_pattern = parsed.path or "/"
        try:
            self._driver.execute_query(
                MARK_ELEMENT_EXPLORED,
                url_pattern=url_pattern,
                role=role,
                name=name,
                project=config.NEO4J_PROJECT,
                database_="neo4j",
            )
        except Exception as e:
            logger.debug("标记元素失败: %s", e)

    def record_navigation(self, from_url: str, to_url: str,
                          role: str, name: str):
        """记录页面间导航"""
        if not self._available():
            return
        fp = urlparse(from_url).path or "/"
        tp = urlparse(to_url).path or "/"
        if fp == tp:
            return  # 同页面操作不记录导航
        try:
            self._driver.execute_query(
                RECORD_NAVIGATION,
                from_pattern=fp,
                to_pattern=tp,
                role=role,
                name=name,
                project=config.NEO4J_PROJECT,
                database_="neo4j",
            )
        except Exception as e:
            logger.debug("记录导航失败: %s", e)

    def record_api(self, full_url: str, role: str, name: str,
                   api_url: str, method: str = "GET"):
        """记录元素触发的 API"""
        if not self._available():
            return
        parsed = urlparse(full_url)
        url_pattern = parsed.path or "/"
        parsed_api = urlparse(api_url)
        clean_api = parsed_api.path
        if parsed_api.query:
            clean_api += "?" + parsed_api.query
        try:
            self._driver.execute_query(
                RECORD_API,
                url_pattern=url_pattern,
                role=role,
                name=name,
                method=method,
                api_url=clean_api,
                project=config.NEO4J_PROJECT,
                database_="neo4j",
            )
        except Exception as e:
            logger.debug("记录 API 失败: %s", e)

    # ---------- 查询 ----------

    def get_graph_summary(self) -> str:
        """获取图摘要，供 Planner 提示词使用（纯文本）"""
        if not self._available():
            return ""

        pages = []
        try:
            result = self._driver.execute_query(QUERY_GRAPH_SUMMARY,
                                                  project=config.NEO4J_PROJECT,
                                                  database_="neo4j")
            for r in result.records:
                pages.append(r.get("page_info") or {})
        except Exception:
            pass

        navs = []
        try:
            result = self._driver.execute_query(QUERY_NAVIGATION_PATHS,
                                                  project=config.NEO4J_PROJECT,
                                                  database_="neo4j")
            for r in result.records:
                navs.append({
                    "from": r.get("from_page"),
                    "to": r.get("to_page"),
                    "via_role": r.get("via_role"),
                    "via_name": r.get("via_name"),
                    "count": r.get("traverse_count"),
                })
        except Exception:
            pass

        lines = ["[知识图谱] 目前已探索的页面结构（可能不完整，仅供参考）："]

        if not pages:
            lines.append("  (尚无探索记录)")
        else:
            for p in pages:
                pat = p.get("url_pattern", "/")
                title = p.get("title", "")
                total = p.get("total_elements", 0)
                label = f"{pat} [{title}]" if title else pat
                lines.append(f"\n  {label}（共 {total} 个元素）")

                # 区域
                sections = p.get("sections", [])
                for s in sections:
                    if s.get("name"):
                        tag = s.get("tag", "")
                        cnt = s.get("element_count", 0)
                        lines.append(f"    ├── {s['name']} ({tag}) — {cnt} 个元素")

                # 未归组元素
                ungr = p.get("ungrouped", 0)
                if ungr > 0:
                    lines.append(f"    └── 其他 — {ungr} 个元素")

        if navs:
            lines.append(f"\n  页面间导航（共 {len(navs)} 条）：")
            details = {}
            for n in navs:
                details.setdefault(n["from"], []).append(n)
            for from_page, paths in details.items():
                for nd in paths:
                    lines.append(f"    {from_page} → [{nd['via_role']}] '{nd['via_name']}' → {nd['to']}")

        lines.append("\n  以上信息仅供参考，请基于用户实际测试场景自行判断，"
                      "不要被已探索的范围限制。")
        return "\n".join(lines)


# ================================================================
# 全局单例
# ================================================================

_kg: Optional[KnowledgeGraph] = None


def get_kg() -> KnowledgeGraph:
    global _kg
    if _kg is None:
        _kg = KnowledgeGraph(
            uri=config.NEO4J_URI,
            user=config.NEO4J_USER,
            password=config.NEO4J_PASSWORD,
        )
        _kg.connect()
    return _kg
