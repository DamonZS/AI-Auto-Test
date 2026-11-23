from pathlib import Path


def test_desktop_worker_token_sources(tmp_path, monkeypatch):
    monkeypatch.syspath_prepend(str(Path(__file__).resolve().parents[1] / "packaging"))
    from desktop import worker_token

    monkeypatch.delenv("PLATFORM_WORKER_TOKEN", raising=False)
    assert worker_token(tmp_path) == ""

    (tmp_path / "desktop-worker-token.txt").write_text("managed-token\n", encoding="utf-8")
    assert worker_token(tmp_path) == "managed-token"

    manual_file = tmp_path / "worker-token.txt"
    manual_file.write_text("manual-token\n", encoding="utf-8-sig")
    assert worker_token(tmp_path) == "manual-token"

    monkeypatch.setenv("PLATFORM_WORKER_TOKEN", " env-token ")
    assert worker_token(tmp_path) == "env-token"

    monkeypatch.setenv("PLATFORM_WORKER_TOKEN", " ")
    manual_file.write_text(" \n", encoding="utf-8-sig")
    assert worker_token(tmp_path) == "managed-token"
