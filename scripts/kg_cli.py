"""Neo4j 知识图谱调试 CLI"""
import argparse
import sys
import json
from auto_ai.knowledge_graph import get_kg


def _colored(text: str, code: int) -> str:
    return f"\033[{code}m{text}\033[0m"


def cmd_pages(kg):
    """列出所有页面"""
    with kg._driver.session(database="neo4j") as s:
        pages = s.run(
            "MATCH (p:Page {project: $p}) RETURN p.url_pattern AS url, p.title AS title, "
            "p.total_elements AS total, p.explored_elements AS explored "
            "ORDER BY p.url_pattern",
            p="auto_ai"
        ).data()
    if not pages:
        print("  暂无页面数据")
        return
    for pg in pages:
        cov = f"{pg['explored']}/{pg['total']}" if pg['total'] else "-"
        print(f"  {_colored(pg['url'], 34)} {_colored('['+pg['title']+']', 90) if pg['title'] else ''} — 元素 {cov}")


def cmd_elements(kg, url_filter: str = None):
    """列出页面上的元素（按区域分组）"""
    with kg._driver.session(database="neo4j") as s:
        pages = s.run(
            "MATCH (p:Page {project: $p}) WHERE $url IS NULL OR p.url_pattern CONTAINS $url "
            "RETURN p.url_pattern AS url ORDER BY p.url_pattern",
            p="auto_ai", url=url_filter
        ).data()

    for pg in pages:
        url = pg["url"]
        print(f"\n{_colored('='*50, 90)}")
        print(f"  {_colored(url, 34)}")
        print(f"{_colored('='*50, 90)}")

        with kg._driver.session(database="neo4j") as s:
            sections = s.run(
                "MATCH (s:Section {project: $p})-[:PART_OF]->(:Page {url_pattern: $u, project: $p}) "
                "RETURN s.name AS name, s.tag AS tag, s.element_count AS cnt ORDER BY s.tag, s.name",
                p="auto_ai", u=url
            ).data()

            for sec in sections:
                print(f"  {_colored('['+sec['tag']+']', 33)} {sec['name']} ({sec['cnt']}个)")

            ungrouped = s.run(
                "MATCH (e:Element {project: $p})-[:BELONGS_TO]->(:Page {url_pattern: $u, project: $p}) "
                "WHERE NOT (e)-[:IN_SECTION]->() "
                "RETURN e.role AS role, e.name AS name, e.explored AS exp, e.locator AS loc "
                "ORDER BY e.role, e.name",
                p="auto_ai", u=url
            ).data()

            if ungrouped:
                print(f"  {_colored('[其他]', 33)} ({len(ungrouped)}个):")
                for el in ungrouped:
                    mark = _colored("[✓]", 32) if el["exp"] else "[ ]"
                    loc = _colored(f"  // {el['loc']}", 90) if el.get("loc") else ""
                    print(f"    {mark} [{el['role']}] {el['name']}{loc}")


def cmd_nav(kg):
    """列出页面间导航路径"""
    with kg._driver.session(database="neo4j") as s:
        navs = s.run(
            "MATCH (a:Page {project: $p})-[r:NAVIGATES_TO]->(b:Page {project: $p}) "
            "RETURN a.url_pattern AS f, b.url_pattern AS t, r.via_role AS role, r.via_name AS name, r.count AS cnt, "
            "r.status AS status ORDER BY r.count DESC",
            p="auto_ai"
        ).data()
    if not navs:
        print("  暂无导航路径")
        return
    for n in navs:
        status = _colored(f"[{n['status']}]", 32 if n["status"] and "200" in str(n["status"]) else 31) if n.get("status") else ""
        print(f"  {n['f']} {_colored('─['+n['name']+']→', 33)} {n['t']} 经过{n['cnt']}次 {status}")


def cmd_apis(kg, url_filter: str = None):
    """列出元素触发的 API 调用"""
    with kg._driver.session(database="neo4j") as s:
        apis = s.run(
            "MATCH (e:Element {project: $p})-[r:TRIGGERS_API]->(a:API {project: $p}) "
            "WHERE $url IS NULL OR a.url CONTAINS $url "
            "RETURN e.name AS el_name, e.role AS el_role, a.url AS api_url, a.method AS method, "
            "a.status AS status, r.count AS cnt ORDER BY r.count DESC",
            p="auto_ai", url=url_filter
        ).data()
    if not apis:
        print("  暂无 API 调用记录")
        return
    for a in apis:
        status = _colored(f"[{a['status']}]", 32 if a["status"] and "200" in str(a["status"]) else 31) if a.get("status") else ""
        print(f"  [{a['el_role']}] {a['el_name']} → {a['method']} {a['api_url']} {status} ({a['cnt']}次)")


def cmd_stats(kg):
    """整体统计"""
    with kg._driver.session(database="neo4j") as s:
        stats = s.run("""
            MATCH (n {project: $p})
            RETURN
                sum(CASE WHEN n:Page THEN 1 ELSE 0 END) AS pages,
                sum(CASE WHEN n:Section THEN 1 ELSE 0 END) AS sections,
                sum(CASE WHEN n:Element THEN 1 ELSE 0 END) AS elements,
                sum(CASE WHEN n:Element AND n.explored = true THEN 1 ELSE 0 END) AS explored,
                sum(CASE WHEN n:API THEN 1 ELSE 0 END) AS apis
        """, p="auto_ai").data()[0]
        navs = s.run(
            "MATCH (:Page {project: $p})-[r:NAVIGATES_TO]->(:Page {project: $p}) RETURN count(r) AS cnt",
            p="auto_ai"
        ).data()[0]["cnt"]
        api_edges = s.run(
            "MATCH (:Element {project: $p})-[r:TRIGGERS_API]->(:API {project: $p}) RETURN count(r) AS cnt",
            p="auto_ai"
        ).data()[0]["cnt"]

    print(f"  Page:    {stats['pages']}")
    print(f"  Section: {stats['sections']}")
    print(f"  Element: {stats['elements']} (已探索: {stats['explored']})")
    print(f"  API:     {stats['apis']}")
    print(f"  导航边:  {navs}")
    print(f"  API边:   {api_edges}")


def cmd_clear(kg, confirmed=False):
    """清空本项目数据"""
    if not confirmed:
        print("  加 --confirmed 确认清空")
        return
    with kg._driver.session(database="neo4j") as s:
        r = s.run("MATCH (n {project: $p}) DETACH DELETE n RETURN count(n) AS n", p="auto_ai").data()
        print(f"  已清空 {r[0]['n']} 个节点")


def main():
    parser = argparse.ArgumentParser(description="Neo4j 知识图谱调试工具")
    parser.add_argument("cmd", nargs="?", default="all",
                        choices=["pages", "elements", "nav", "apis", "stats", "all", "clear"],
                        help="查询命令")
    parser.add_argument("--url", help="过滤指定 URL（模糊匹配）")
    parser.add_argument("--confirmed", action="store_true", help="确认执行（clear 时需要）")
    args = parser.parse_args()

    kg = get_kg()
    kg.connect()

    if not kg._available():
        print(_colored("Neo4j 未连接，请检查 NEO4J_ENABLED 和连接配置", 31))
        sys.exit(1)

    if args.cmd == "clear":
        cmd_clear(kg, args.confirmed)
        kg.close()
        return

    if args.cmd in ("all", "stats"):
        print(f"\n{_colored('◆ 整体统计', 36)}")
        cmd_stats(kg)

    if args.cmd in ("all", "pages"):
        print(f"\n{_colored('◆ 页面列表', 36)}")
        cmd_pages(kg)

    if args.cmd in ("all", "elements"):
        print(f"\n{_colored('◆ 页面元素', 36)}")
        cmd_elements(kg, args.url)

    if args.cmd in ("all", "nav"):
        print(f"\n{_colored('◆ 导航路径', 36)}")
        cmd_nav(kg)

    if args.cmd in ("all", "apis"):
        print(f"\n{_colored('◆ API 调用', 36)}")
        cmd_apis(kg, args.url)

    kg.close()


if __name__ == "__main__":
    main()
