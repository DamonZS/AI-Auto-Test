# 需求导入、修订与版本影响

以下接口使用工作台现有 Bearer 会话和 `X-Tenant-Id`。租户成员可读，只有 owner/editor 可写。所有项目、需求、项目版本和受影响用例均按租户及项目匹配。

```text
项目版本 ── 关联 ──> 需求修订 ── 上一修订 ──> 变更前内容
                         │
                         └── 所属需求 <── cases.requirement_id
```

## 接口

| 方法与路径 | 请求 | 成功响应 |
| --- | --- | --- |
| `POST /api/requirements/import` | JSON `project_id`、`filename`、`text`，均为必填字符串。Markdown/TXT 可选 `code`、`title` | `201` `{"ids":["..."]}`，按导入顺序返回 |
| `PATCH /api/requirements/{id}` | JSON 至少提交一个字符串字段 `code`、`title`、`content` | `200`，返回新增的修订快照 |
| `GET /api/requirements/{id}/history` | 无请求体 | `200`，返回该需求的修订快照列表，按 `revision_no` 升序 |
| `PUT /api/requirements/{id}/revisions/{revision}/versions/{versionId}` | 无请求体。`revision` 为正整数 | `204`。相同关联重复提交效果相同 |
| `GET /api/versions/{id}/requirement-impact` | 无请求体 | `200` `{"affected_case_count":0,"changes":[]}` |

快照包含 `revision_no`（从 1 开始的整数）、`code`、`title`、`content`、`created_at`（SQLite UTC 时间字符串）。每条 `changes` 含 `requirement_id`、`previous`（上一修订快照或 `null`）、`current`（已关联的快照）和 `affected_case_count`。后续编辑不会改变已关联的 `current`。同一需求的多条修订允许关联同一项目版本，版本总数按当前关联用例去重。首版修订的 `previous` 为 `null`。无需求关联的有效项目版本返回空列表。

## 导入和修订规则

- 导入接受 UTF-8，可带 BOM，文本上限为 1 MiB。`.csv` 使用 `code,title,content` 表头，支持 CSV 引号及多行字段。每行创建一条需求，标题必填，编号和内容可为空。任意行不合法时整批不写入。
- `.md`、`.markdown` 和 `.txt` 各创建一条需求。内容为原文（去掉 BOM），标题默认取文件名去掉扩展名。空白文件无效。
- 导入写入新需求和初始修订。编号沿用旧接口的规则，可以重复。试用时可在需求页载入示例 CSV，点击导入后写入当前选择的项目。
- 编辑保留需求 ID、项目、状态和创建时间。每次有效的 `PATCH` 追加一条快照，省略的字段沿用当前值。标题去掉首尾空白后不能为空。
- 受影响用例按当前 `cases.requirement_id` 统计，包含该租户及项目下所有状态的直接关联用例。

错误响应为 `{"error":"..."}`。`400` 表示请求或文件格式错误；`401` 表示未认证；`403` 表示无权限；`404` 表示目标不存在或不属于当前租户及项目；`413` 表示导入超过大小限制；`500` 表示存储错误。

## 主控接入

- 在 `api.Server.Routes()` 中调用 `s.registerRequirements(mux)`。现有 `GET /api/requirements` 与 `POST /api/requirements` 路由继续由 `domain.go` 的处理器提供。
- 在现有 schema 和迁移之后调用 `store.MigrateRequirements(db)`，按现有启动流程处理错误。迁移可重复运行，只新增修订表、版本关联表和插入触发器，并为已有合法需求补首版快照。已有需求行原样保留。旧手工创建入口通过触发器保存首版快照，跨租户项目插入仍按旧处理器的存储错误响应返回。
- `RequirementsView` 的「生成用例」发出 `generate-cases` 事件，载荷为 `{ projectId, requirementId, versionId }`。`versionId` 取需求所在项目当前选中的版本，未选时为 `undefined`。
