import json
import subprocess
import sys

import pytest

from agents.knowledge_retrieval import retrieve_requirements


def knowledge(*requirements, tenant="tenant-a", project="project-a"):
    return {
        "tenant_id": tenant,
        "project_id": project,
        "requirements": [
            {"id": doc_id, "title": title, "content": content}
            for doc_id, title, content in requirements
        ],
    }


def test_disabled_or_missing_snapshot_does_not_create_index(tmp_path):
    assert retrieve_requirements(
        knowledge(("req-1", "Sign in", "Use MFA")), query="MFA", persist_dir=None
    ) == []
    store = tmp_path / "chroma"
    assert retrieve_requirements(None, query="MFA", persist_dir=store) == []
    assert not store.exists()


@pytest.fixture
def chroma_store(tmp_path):
    chromadb = pytest.importorskip("chromadb")
    from chromadb.config import Settings

    path = tmp_path / "chroma"
    client = chromadb.PersistentClient(path=str(path), settings=Settings(anonymized_telemetry=False))
    return path, client


def test_ranks_chinese_requirements_and_chunks_long_documents(chroma_store):
    store, _ = chroma_store
    context = knowledge(
        ("search", "搜索", "搜索结果按日期排序。"),
        ("login", "账号安全", "背景" * 700 + "密码重置后需要重新登录。"),
    )

    assert retrieve_requirements(context, query="密码重置后重新登录", persist_dir=store, limit=1) == [
        {
            "requirement_id": "login",
            "title": "账号安全",
            "content": "背景" * 100 + "密码重置后需要重新登录。",
            "chunk_index": 1,
        }
    ]
    assert retrieve_requirements(context, query="---", persist_dir=store) == []


def test_uses_only_authorized_scope_and_persists_filtered_rows(chroma_store):
    store, client = chroma_store
    current = knowledge(("mine", "Billing CSV", "Billing CSV is enabled."))
    other_project = knowledge(
        ("other-project", "Billing CSV", "Another project rule."), project="project-b"
    )
    other_tenant = knowledge(
        ("other-tenant", "Billing CSV", "Another tenant rule."), tenant="tenant-b"
    )
    for context in (other_project, other_tenant, current):
        assert [hit["requirement_id"] for hit in retrieve_requirements(
            context, query="billing csv", persist_dir=store
        )] == [context["requirements"][0]["id"]]

    collections = client.list_collections()
    assert len(collections) == 2
    assert sorted(collection.count() for collection in collections) == [1, 2]
    scopes = {
        (metadata["tenant_id"], metadata["project_id"])
        for collection in collections
        for metadata in collection.get(include=["metadatas"])["metadatas"]
    }
    assert scopes == {
        ("tenant-a", "project-a"), ("tenant-a", "project-b"), ("tenant-b", "project-a")
    }


def test_sync_updates_removes_and_clears_only_the_current_project(chroma_store):
    store, client = chroma_store
    original = knowledge(
        ("req-1", "Export", "Spreadsheet download"),
        ("req-2", "Export", "Spreadsheet preview"),
    )
    sibling = knowledge(("req-3", "Export", "Spreadsheet print"), project="project-b")
    updated = knowledge(("req-1", "Export", "Spreadsheet upload"))
    for context in (original, sibling):
        retrieve_requirements(context, query="Spreadsheet", persist_dir=store)

    for _ in range(2):
        hits = retrieve_requirements(updated, query="Spreadsheet", persist_dir=store)
        assert [(hit["requirement_id"], hit["content"]) for hit in hits] == [
            ("req-1", "Spreadsheet upload")
        ]

    (collection,) = client.list_collections()
    assert collection.count() == 2
    assert retrieve_requirements(knowledge(), query="Spreadsheet", persist_dir=store) == []
    assert collection.count() == 1
    assert retrieve_requirements(sibling, query="Spreadsheet", persist_dir=store)[0][
        "requirement_id"
    ] == "req-3"


def test_persistent_index_can_be_queried_in_a_fresh_process(chroma_store):
    store, _ = chroma_store
    context = knowledge(("req-1", "Checkout", "Receipt is displayed after checkout."))
    hits = retrieve_requirements(context, query="checkout", persist_dir=store)
    result = subprocess.run(
        [
            sys.executable,
            "-c",
            "import chromadb, json, sys; from chromadb.config import Settings; "
            "from agents.knowledge_retrieval import retrieve_requirements; "
            "client = chromadb.PersistentClient(path=sys.argv[2], "
            "settings=Settings(anonymized_telemetry=False)); "
            "assert [item.count() for item in client.list_collections()] == [1]; "
            "print(json.dumps(retrieve_requirements(json.loads(sys.argv[1]), "
            "query='checkout', persist_dir=sys.argv[2])))",
            json.dumps(context),
            str(store),
        ],
        check=True,
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert hits and json.loads(result.stdout) == hits
