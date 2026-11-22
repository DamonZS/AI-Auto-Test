from collections.abc import Mapping
from dataclasses import dataclass


@dataclass(frozen=True)
class ReportQualityResult:
    status: str
    missing_sections: list[str]
    requires_human_review: bool
    review_approved: bool


def check_report_completeness(report: Mapping[str, object]) -> ReportQualityResult:
    statistics = report.get("case_statistics")
    coverage = report.get("requirement_coverage")
    defects = report.get("defect_distribution")
    conclusion = report.get("conclusion")
    signoff = report.get("review_signoff")
    risk_level = report.get("risk_level")

    valid_defects = isinstance(defects, Mapping) and all(
        _is_count(count) for count in defects.values()
    )
    valid_signoff = isinstance(signoff, Mapping) and signoff.get("status") in (
        "not_required", "pending", "approved", "rejected"
    )
    review_approved = bool(
        valid_signoff and signoff["status"] == "approved" and _has_text(signoff.get("reviewer"))
    )
    requires_review = (
        (isinstance(risk_level, str) and risk_level.strip().upper() in ("P0", "P1"))
        or (isinstance(defects, Mapping) and any(
            isinstance(level, str) and level.strip().upper() in ("P0", "P1")
            and _is_count(count) and count > 0
            for level, count in defects.items()
        ))
        or (isinstance(conclusion, str) and "上线" in conclusion)
    )
    checks = {
        "test_scope_environment": _has_text(report.get("test_scope_environment")),
        "case_statistics": _valid_statistics(statistics),
        "requirement_coverage": _has_counts(coverage, "covered", "total")
        and coverage["covered"] <= coverage["total"],
        "execution_rate": _is_rate(report.get("execution_rate")),
        "pass_rate": _is_rate(report.get("pass_rate")),
        "defect_distribution": valid_defects,
        "residual_risks": _has_text(report.get("residual_risks")),
        "conclusion": _has_text(conclusion),
        "review_signoff": valid_signoff and (signoff["status"] != "approved" or review_approved),
    }
    missing_sections = [section for section, valid in checks.items() if not valid]
    status = "complete"
    if missing_sections:
        status = "incomplete"
    elif requires_review and not review_approved:
        status = "review_required"
    return ReportQualityResult(status, missing_sections, requires_review, review_approved)


def _has_text(value):
    return isinstance(value, str) and bool(value.strip())


def _is_count(value):
    return type(value) is int and value >= 0


def _has_counts(value, *fields):
    return isinstance(value, Mapping) and all(_is_count(value.get(field)) for field in fields)


def _valid_statistics(value):
    return (
        _has_counts(value, "total", "executed", "passed", "failed", "blocked")
        and value["passed"] + value["failed"] + value["blocked"]
        == value["executed"] <= value["total"]
    )


def _is_rate(value):
    return type(value) in (int, float) and 0 <= value <= 1
