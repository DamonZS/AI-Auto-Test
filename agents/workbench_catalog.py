"""Static definitions for AI testing workbench entries.

This module describes entry requirements and workflow order only. It does not
execute models, route requests, or produce test analysis.
"""

from dataclasses import dataclass


@dataclass(frozen=True)
class WorkbenchEntry:
    id: str
    name: str
    required: tuple[str, ...]
    knowledge: tuple[str, ...]
    workflow: tuple[str, ...]


ENTRY_DEFS = (
    WorkbenchEntry(
        id="testcase_gen",
        name="测试用例生成",
        required=("需求背景", "验收标准"),
        knowledge=("业务规则库", "接口异常库", "历史 Bug 库"),
        workflow=(
            "校验输入",
            "拆解测试点",
            "覆盖正常/异常/边界",
            "生成结构化用例",
            "标记人工复核",
        ),
    ),
    WorkbenchEntry(
        id="bug_analysis",
        name="Bug 分析",
        required=("问题标题", "复现步骤", "实际结果", "预期结果", "环境"),
        knowledge=("历史 Bug 库", "日志规律库", "SQL 经验库", "业务规则库"),
        workflow=(
            "校验输入",
            "描述现象",
            "识别影响模块",
            "给出日志方向",
            "给出 SQL 方向",
            "检索历史 Bug",
            "生成回归范围",
            "判断风险",
            "标记人工复核",
        ),
    ),
    WorkbenchEntry(
        id="log_triage",
        name="日志排查",
        required=("日志或 traceId", "时间范围", "运行环境"),
        knowledge=("日志规律库", "历史 Bug 库"),
        workflow=(
            "提取关键字",
            "识别异常",
            "还原调用链",
            "定位可疑模块",
            "匹配历史规律",
            "给出排查建议",
            "标记证据缺口",
        ),
    ),
    WorkbenchEntry(
        id="sql_analysis",
        name="SQL 分析",
        required=("SQL 文本", "分析目标"),
        knowledge=("SQL 经验库", "压测经验库"),
        workflow=(
            "识别风险",
            "检查 WHERE",
            "给出索引验证方向",
            "分析慢查询",
            "检查 JOIN",
            "检查 ORDER BY",
            "判断性能风险",
            "建议 EXPLAIN 验证",
        ),
    ),
    WorkbenchEntry(
        id="regression_list",
        name="回归清单",
        required=("版本或变更内容", "影响模块"),
        knowledge=("回归规则库", "历史 Bug 库"),
        workflow=(
            "识别直接变更",
            "扩展上下游",
            "还原业务链路",
            "检索历史 Bug",
            "评估风险",
            "生成必测/建议项",
            "人工复核",
        ),
    ),
    WorkbenchEntry(
        id="test_report",
        name="测试报告",
        required=("版本", "测试范围", "执行结果"),
        knowledge=("业务规则库", "历史 Bug 库", "回归规则库"),
        workflow=(
            "汇总测试范围",
            "统计执行结果",
            "汇总缺陷",
            "识别风险模块",
            "整理遗留问题",
            "生成上线建议",
            "人工审批",
        ),
    ),
    WorkbenchEntry(
        id="prompt_test",
        name="Prompt 测试",
        required=("Prompt 文本", "预期输出或评价标准"),
        knowledge=(),
        workflow=(
            "读取 Prompt",
            "准备样例",
            "逐项运行",
            "对照 Schema",
            "检查事实与幻觉",
            "记录差异",
            "人工判定",
        ),
    ),
)

DEFAULT_ORDER = tuple(entry.id for entry in ENTRY_DEFS)
_ENTRIES_BY_ID = {entry.id: entry for entry in ENTRY_DEFS}


def get_entry(entry_id: str) -> WorkbenchEntry | None:
    """Return a workbench entry by its stable ID."""
    return _ENTRIES_BY_ID.get(entry_id)


def requires_human_review(risk_level: str, entry_id: str) -> bool:
    """Return the catalog review gate for a risk level and entry."""
    return risk_level.upper() in {"P0", "P1"} or entry_id == "prompt_test"
