import json
from pathlib import Path
import subprocess
import sys

import pytest


SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "case_cli.py"


def run_cli(tmp_path, *args, stdin=None):
    return subprocess.run(
        [sys.executable, str(SCRIPT), *args], cwd=tmp_path,
        input=stdin, capture_output=True, encoding="utf-8", timeout=10,
    )


def test_recording_workbook_round_trip(tmp_path):
    events = tmp_path / "events.json"
    events.write_text('[{"action":"click","selector":"role=button[name=\\"登录\\"]"}]', encoding="utf-8")
    converted = run_cli(tmp_path, "convert-recorded", events.name)
    assert converted.returncode == 0, converted.stderr
    assert json.loads(converted.stdout) == {
        "steps": [{"action": "click", "role": "button", "name": "登录"}],
    }

    exported = run_cli(tmp_path, "export-xlsx", "-", "case.xlsx", stdin=converted.stdout)
    assert exported.returncode == 0, exported.stderr
    assert exported.stdout == ""

    imported = run_cli(tmp_path, "import-xlsx", "case.xlsx", "scenario.json")
    assert imported.returncode == 0, imported.stderr
    assert json.loads((tmp_path / "scenario.json").read_text(encoding="utf-8")) == json.loads(converted.stdout)


@pytest.mark.parametrize("stdin", ["{", "{}", '[{"action":"scroll"}]'])
def test_invalid_recording_reports_error_on_stderr(tmp_path, stdin):
    result = run_cli(tmp_path, "convert-recorded", "-", stdin=stdin)
    assert result.returncode == 2
    assert result.stdout == ""
    assert "error:" in result.stderr
    assert "Traceback" not in result.stderr
