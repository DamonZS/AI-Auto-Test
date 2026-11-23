# 项目工作台接口

以下接口沿用工作区 Bearer 会话和 `X-Tenant-Id`。工作区成员可以读取，编辑者和所有者可以写入。响应使用现有 JSON 错误格式。未登录返回 `401`，权限不足返回 `403`，目标资源不在对应的工作区或项目内返回 `404`，输入无效返回 `400`。

| 方法与路径 | 请求 | 成功响应 |
|---|---|---|
| `GET /api/projects/{id}` | 项目 ID | `200`，现有项目字段及 `metrics` |
| `PATCH /api/projects/{id}` | 可选 `name`、`base_url`、`allowed_hosts` | `204` |
| `POST /api/projects/{id}/environments` | 必填 `name`、`base_url`，可选 `source`、`level`、`version_id` | `201`，`{"id":"…"}` |
| `PATCH /api/environments/{id}` | 可选 `name`、`base_url`、`source`、`level`、`version_id` | `204` |

字段均为字符串。`name` 提交时须非空，`base_url` 须为 HTTP(S) URL。PATCH 中省略的字段保持原值。`source` 可为 `platform` 或 `mcp`，创建时默认 `platform`。`level` 可为 `development`、`test`、`staging`、`production` 或空字符串。`version_id` 只能引用同项目的已登记版本。传空字符串可清除级别或版本。环境 `status` 保留现有含义。

`metrics` 包含 `requirements`、`cases`、`approved_cases`、`versions`、`environments`、`runs`、`passed_runs`、`open_bugs`。每项只统计当前项目。执行记录通过所属用例关联项目。项目页用 `approved_cases / cases` 展示用例审批进度，零用例时显示暂无进度。

项目创建、编辑、列表和详情支持 `code`（项目代号）、`lead`（负责人）、`description`（项目说明）和 `lifecycle`（`planning`、`active`、`completed`、`archived`）。已归档项目可查看，列表和项目选择器保留历史项目。旧项目默认为进行中。

`GET /api/environments` 保留现有字段并追加 `level` 与 `version_id`。旧环境默认两项均为空。原有环境创建接口继续可用。版本页使用现有 `GET /api/versions/{id}/requirement-impact` 比较两版各需求最新关联的修订，展示关联新增、更新或移除。版本比较使用已登记的提交引用、影响范围和需求关联。

## 接线

在 `Server.Routes()` 调用 `s.registerProjectWorkspace(mux)`。在 `store.Open` 完成基础表创建后调用 `MigrateProjectWorkspace(db)` 并传播错误。迁移只为 `environments` 增加两列，可以重复运行。

`ProjectsView`、`VersionsView`、`EnvironmentsView` 接收可选 `projectId` 字符串。`ProjectsView` 发出 `open-project(projectId)` 与 `open-project-view({ projectId, view })`，其中 `view` 取 `requirements`、`versions`、`environments`、`cases`、`runs`、`bugs`。
项目保存后发出 `projects-changed`，供全局项目选择器刷新。
