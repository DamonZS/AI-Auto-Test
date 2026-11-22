# UI 需求知识检索

## Heartbeat 数据约定

控制面在已领取的 UI run 上可附加 `knowledge_context`。`tenant_id` 取已认证 worker 所领 run 的租户，`project_id` 取该 run 对应用例的项目。SQLite `requirements` 表是唯一来源。按这两个字段筛选当前已保存的需求，现有保存入口写入的状态是 `open`。

```sql
SELECT id, title, content FROM requirements
WHERE tenant_id = ? AND project_id = ? AND status = 'open'
ORDER BY id;
```

```json
{
  "knowledge_context": {
    "tenant_id": "tenant-a",
    "project_id": "project-a",
    "requirements": [
      {"id": "req-1", "title": "账号安全", "content": "密码重置后需要重新登录。"}
    ]
  }
}
```

`requirements` 是该项目的完整快照。查询成功且没有符合条件的需求时传 `[]`，用于清理该项目的索引。关闭知识检索或控制面读取失败时省略整个 `knowledge_context`。字段由控制面生成，排除用例草稿、生成请求提交的源码、测试数据、环境凭据和其他工作台文档。

## `ui_runner` 接入

在自然语言 UI 路径中，将下列同步函数交给 `asyncio.to_thread`。传入用例的场景文本作为查询，调用时将 `persist_dir` 设为 worker 管理的索引目录或 `None`。

```python
from agents.knowledge_retrieval import retrieve_requirements

hits = await asyncio.to_thread(
    retrieve_requirements,
    task.get("knowledge_context"),
    query=task["scenario"],
    persist_dir=knowledge_dir,
    limit=3,
)
```

结果按相关度排序，每项提供 `requirement_id`、`title`、`content` 和从零开始的 `chunk_index`。运行器可将片段作为需求参考数据引用，测试结果仍由原有页面断言决定。索引不可用时，由运行器记录简短的降级状态并继续执行。缺少 Chroma 或索引操作失败时，函数向调用方抛出异常。传入 `None` 作为目录或快照时直接返回 `[]`。

## 部署和验证

设置 `PLATFORM_KNOWLEDGE_DIR` 后，worker 通过 `POST /api/runs/heartbeat?knowledge=requirements` 请求需求快照。

Chroma 默认为关闭。启用时，在 worker 的 Python 环境安装 `requirements-knowledge.txt`，为每个 worker 配置独立、持久的本地目录。Windows x64 上用 Python 3.13 验证了 `chromadb==1.5.9`。向量使用本地确定性词项哈希，覆盖英文词项和中文双字匹配，无运行时模型下载。它提供字面相关度排序。

Chroma 对每个租户使用独立 collection，读取、查询和删除均按 `tenant_id`、`project_id` 过滤。索引保存向量、作用域和片段哈希，命中文本取自当前授权快照。项目索引随该项目下一次完整快照更新。回退时将目录配置置空。索引可从 SQLite 快照重建。

```powershell
python -m pip install -r requirements-knowledge.txt
python -m pytest -q tests/test_knowledge_retrieval.py
```
