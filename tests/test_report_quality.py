import pytest

from agents.report_quality import check_report_completeness


def complete_report(**overrides):
    report = {
        "risk_level": "P2",
        "test_scope_environment": "staging，支付回调与订单同步，v2.8.3",
        "case_statistics": {"total": 10, "executed": 10, "passed": 9, "failed": 1, "blocked": 0},
        "requirement_coverage": {"covered": 8, "total": 8},
        "execution_rate": 1.0,
        "pass_rate": 0.9,
        "defect_distribution": {"P1": 0, "P2": 2},
        "residual_risks": "无",
        "conclusion": "本轮测试已覆盖计划内范围",
        "review_signoff": {"status": "not_required"},
    }
    report.update(overrides)
    return report


def test_complete_report_is_complete_without_review_gate():
    result = check_report_completeness(complete_report())

    assert result.status == "complete"
    assert result.missing_sections == []
    assert result.requires_human_review is False


def test_missing_required_chapters_are_reported():
    report = complete_report()
    report.pop("defect_distribution")
    report.pop("review_signoff")

    result = check_report_completeness(report)

    assert result.status == "incomplete"
    assert result.missing_sections == ["defect_distribution", "review_signoff"]


@pytest.mark.parametrize("risk_level", ["P0", "p0", "P1", "p1"])
def test_p0_and_p1_require_human_review(risk_level):
    result = check_report_completeness(
        complete_report(
            risk_level=risk_level,
            review_signoff={"reviewer": "qa-owner", "status": "pending"},
        )
    )

    assert result.status == "review_required"
    assert result.requires_human_review is True


@pytest.mark.parametrize("conclusion", ["建议上线", "有条件上线", "暂缓上线"])
def test_release_conclusion_requires_human_review(conclusion):
    result = check_report_completeness(
        complete_report(
            conclusion=conclusion,
            review_signoff={"reviewer": "qa-owner", "status": "pending"},
        )
    )

    assert result.status == "review_required"
    assert result.requires_human_review is True


def test_human_approval_completes_a_report_that_requires_review():
    result = check_report_completeness(
        complete_report(
            risk_level="P0",
            conclusion="建议上线",
            review_signoff={"reviewer": "qa-owner", "status": "approved"},
        )
    )

    assert result.status == "complete"
    assert result.requires_human_review is True
    assert result.review_approved is True


def test_invalid_metric_values_make_report_incomplete():
    result = check_report_completeness(
        complete_report(
            case_statistics={"total": 10, "executed": 11, "passed": 9, "failed": 1, "blocked": 0},
            requirement_coverage={"covered": 9, "total": 8},
            execution_rate=1.1,
            pass_rate=-0.1,
        )
    )

    assert result.status == "incomplete"
    assert result.missing_sections == [
        "case_statistics",
        "requirement_coverage",
        "execution_rate",
        "pass_rate",
    ]


@pytest.mark.parametrize("level", ["P0", "p1"])
def test_high_priority_defect_requires_review_even_with_low_report_risk(level):
    result = check_report_completeness(complete_report(defect_distribution={level: 1}))

    assert result.status == "review_required"
    assert result.requires_human_review is True


def test_approval_without_named_reviewer_is_incomplete():
    result = check_report_completeness(
        complete_report(conclusion="建议上线", review_signoff={"status": "approved", "reviewer": " "})
    )

    assert result.status == "incomplete"
    assert result.missing_sections == ["review_signoff"]
    assert result.requires_human_review is True
    assert result.review_approved is False


@pytest.mark.parametrize("section, value", [
    ("test_scope_environment", " "),
    ("case_statistics", {"total": 10, "executed": 9, "passed": 9, "failed": 1, "blocked": 0}),
    ("requirement_coverage", {"covered": True, "total": 1}),
    ("execution_rate", float("nan")),
    ("pass_rate", "0.9"),
    ("defect_distribution", {"P2": -1}),
    ("residual_risks", None),
    ("conclusion", ""),
])
def test_invalid_section_is_reported(section, value):
    result = check_report_completeness(complete_report(**{section: value}))

    assert result.status == "incomplete"
    assert result.missing_sections == [section]
