# 缺陷工作台

流程参考禅道的[提交 Bug](https://www.zentao.net/book/zentaopms/133.html)与[确认、解决 Bug](https://www.zentao.net/book/zentaopms/661.html)。沿用平台状态机：新建 → 确认 → 修复中 → 已修复 → 已验证 → 关闭。验证失败或关闭后重现时，使用重新打开，再走确认和修复。拒绝仍按现有流转规则处理。

## 接口与集成

在 `Routes()` 中调用 `s.registerBugManagement(mux)`，由此注册下表中的两个新增路由。列表、创建、流转和复测沿用原路由及成功响应。Vue 继续发出 `open-run(runID)`，查看执行记录传原运行 ID，复测传新运行 ID。

| 路由 | 请求与响应 |
|---|---|
| `GET /api/bugs` | 可组合传 `project_id`、`status`、`severity`、`q`。`q` 按标题和编号做字面子串匹配，英文字母不区分大小写。返回当前租户的匹配缺陷数组。 |
| `POST /api/bugs` | 保留 `project_id`、`title`、`severity`、`priority`、`signature`、`evidence`，可额外传 `repro_steps`、`expected_result`、`actual_result`、`assignee`、`discovered_version_id`。返回 `{id, bug_code}`。 |
| `GET /api/bugs/{id}` | 返回缺陷字段和 `project_name`、`discovered_version_name`、`fixed_version_name`、`available_transitions`、`history`。历史按事件从新到旧，字段为 `id`、`from_status`、`to_status`、`actor`、`note`、`at`。 |
| `PATCH /api/bugs/{id}` | 可更新标题、级别、优先级、证据、复现三项、处理人和发现版本。省略字段时保留原值。发现版本和处理人可传空字符串清除。返回 `{id}`。 |
| `POST /api/bugs/{id}/transition` | 沿用 `to`、`note`、`severity`，设为 `fixed` 时可传 `fixed_version_id`。省略时保留原关联。界面在提交状态流转前要求输入备注，现有 API 调用仍可省略。返回 `{id, status}`。 |
| `POST /api/bugs/{id}/retest` | 沿用源运行关联的定稿用例和参数。返回 `{run_id, status}`，复测结果进入原执行流程。 |

两个版本字段仅关联与缺陷同租户、同项目的 `versions`。无效版本返回 `400`，当前租户下不存在的缺陷详情和编辑返回 `404`，非法流转返回 `409`。处理人是本地文本字段。登录模式下，操作历史的操作人来自会话。

## SQLite 迁移

在 `bugs` 建表定义和现有库增量迁移中添加 `repro_steps`、`expected_result`、`actual_result`、`assignee`、`discovered_version_id`、`fixed_version_id`，均为 `TEXT NOT NULL DEFAULT ''`。既有缺陷及自动创建的缺陷使用空值，可在详情中补录。复用现有 `bug_events`，历史中同状态的事件表示资料编辑。缺陷重开时保留最近一次修复版本，下一次标记为已修复时可以指定新版本或清除关联。
