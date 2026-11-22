"""Tests for the static AI workbench domain catalog."""

import unittest

from agents.workbench_catalog import (
    DEFAULT_ORDER,
    ENTRY_DEFS,
    get_entry,
    requires_human_review,
)


class WorkbenchCatalogTests(unittest.TestCase):
    def test_has_seven_unique_entries_in_stable_default_order(self):
        expected = (
            "testcase_gen",
            "bug_analysis",
            "log_triage",
            "sql_analysis",
            "regression_list",
            "test_report",
            "prompt_test",
        )

        self.assertEqual(expected, DEFAULT_ORDER)
        self.assertEqual(expected, tuple(entry.id for entry in ENTRY_DEFS))
        self.assertEqual(len(expected), len(set(entry.id for entry in ENTRY_DEFS)))

    def test_each_entry_declares_required_fields_knowledge_and_ordered_workflow(self):
        for entry in ENTRY_DEFS:
            with self.subTest(entry=entry.id):
                self.assertTrue(entry.name)
                self.assertTrue(entry.required)
                self.assertTrue(all(field.strip() for field in entry.required))
                self.assertEqual(len(entry.required), len(set(entry.required)))
                self.assertIsInstance(entry.knowledge, tuple)
                self.assertTrue(entry.workflow)
                self.assertTrue(all(step.strip() for step in entry.workflow))

    def test_entry_lookup_returns_definition_or_none(self):
        self.assertEqual("bug_analysis", get_entry("bug_analysis").id)
        self.assertIsNone(get_entry("unknown"))

    def test_p0_and_p1_always_require_human_review(self):
        for risk in ("P0", "p0", "P1", "p1"):
            with self.subTest(risk=risk):
                self.assertTrue(requires_human_review(risk, "bug_analysis"))

    def test_prompt_test_always_requires_human_review(self):
        self.assertTrue(requires_human_review("P2", "prompt_test"))
        self.assertTrue(requires_human_review("P3", "prompt_test"))

    def test_lower_risk_non_prompt_entries_do_not_require_review_by_default(self):
        for risk in ("P2", "P3"):
            with self.subTest(risk=risk):
                self.assertFalse(requires_human_review(risk, "testcase_gen"))


if __name__ == "__main__":
    unittest.main()
