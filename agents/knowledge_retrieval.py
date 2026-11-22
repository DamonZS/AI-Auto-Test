"""Index the control plane's authorized, saved requirements snapshot for a project."""

import hashlib
import json
import re
import unicodedata
from pathlib import Path
from typing import TypedDict


_CHUNK_SIZE = 1200
_DIMENSIONS = 2048
_WORDS = re.compile(r"[\u3400-\u9fff]+|[^\W_\u3400-\u9fff]+")


class SavedRequirement(TypedDict):
    id: str
    title: str
    content: str


class KnowledgeContext(TypedDict):
    tenant_id: str
    project_id: str
    requirements: list[SavedRequirement]


class RequirementHit(TypedDict):
    requirement_id: str
    title: str
    content: str
    chunk_index: int


def retrieve_requirements(
    context: KnowledgeContext | None,
    *,
    query: str,
    persist_dir: str | Path | None,
    limit: int = 3,
) -> list[RequirementHit]:
    """Sync and query an authorized heartbeat snapshot. A missing snapshot leaves the index alone."""
    if persist_dir is None or context is None:
        return []

    import chromadb
    from chromadb.config import Settings

    tenant_id, project_id = context["tenant_id"], context["project_id"]
    scope = {"$and": [{"tenant_id": tenant_id}, {"project_id": project_id}]}
    client = chromadb.PersistentClient(
        path=str(persist_dir), settings=Settings(anonymized_telemetry=False)
    )
    tenant_key = hashlib.sha256(tenant_id.encode()).hexdigest()
    collection = client.get_or_create_collection(
        name=f"requirements-v1-{tenant_key}",
        embedding_function=None,
        configuration={"hnsw": {"space": "cosine"}},
    )
    chunks = _requirement_chunks(project_id, context["requirements"])
    indexed_ids = set(collection.get(where=scope, include=[])["ids"])
    new_ids = [chunk_id for chunk_id in chunks if chunk_id not in indexed_ids]
    batch_size = client.get_max_batch_size()
    for offset in range(0, len(new_ids), batch_size):
        batch = new_ids[offset : offset + batch_size]
        batch_chunks = [chunks[chunk_id] for chunk_id in batch]
        collection.upsert(
            ids=batch,
            embeddings=[_embed(f"{chunk['title']}\n{chunk['content']}") for chunk in batch_chunks],
            metadatas=[{"tenant_id": tenant_id, "project_id": project_id} for _ in batch],
        )
    stale_ids = sorted(indexed_ids.difference(chunks))
    for offset in range(0, len(stale_ids), batch_size):
        collection.delete(ids=stale_ids[offset : offset + batch_size], where=scope)

    query_vector = _embed(query)
    if not chunks or not any(query_vector):
        return []
    result = collection.query(
        query_embeddings=[query_vector],
        ids=list(chunks),
        where=scope,
        n_results=min(limit, len(chunks)),
        include=["distances"],
    )
    return [
        chunks[chunk_id]
        for chunk_id, distance in zip(result["ids"][0], result["distances"][0])
        if distance < 1.0
    ]


def _requirement_chunks(
    project_id: str, requirements: list[SavedRequirement]
) -> dict[str, RequirementHit]:
    chunks: dict[str, RequirementHit] = {}
    for document in requirements:
        content = document["content"].strip()
        for index, offset in enumerate(range(0, max(1, len(content)), _CHUNK_SIZE)):
            chunk = content[offset : offset + _CHUNK_SIZE]
            identity = (project_id, document["id"], index, document["title"], chunk)
            chunk_id = hashlib.sha256(json.dumps(identity, ensure_ascii=False).encode()).hexdigest()
            chunks[chunk_id] = {
                "requirement_id": document["id"],
                "title": document["title"],
                "content": chunk,
                "chunk_index": index,
            }
    return chunks


def _embed(text: str) -> list[float]:
    vector = [0.0] * _DIMENSIONS
    for word in _WORDS.findall(unicodedata.normalize("NFKC", text).casefold()):
        if "\u3400" <= word[0] <= "\u9fff" and len(word) > 1:
            features = (word[index : index + 2] for index in range(len(word) - 1))
        else:
            features = (word,)
        for feature in features:
            bucket = int.from_bytes(hashlib.blake2s(feature.encode(), digest_size=4).digest(), "big")
            vector[bucket % _DIMENSIONS] += 1.0
    return vector
