# 需求功能点工作台

接口使用工作区 Web 会话和 `X-Tenant-Id`。查看轮次允许 `viewer`、`editor` 和 `owner`，生成和评审允许 `editor` 和 `owner`。每个需求修订独立评审。工作台保留原有导入、修订和版本影响入口。

| 方法 | 路径（相对 `/api/requirements/{id}/point-analyses`） | 请求与响应 |
| --- | --- | --- |
| GET | `?revision_no=1` | `200`，返回 `{ "rounds": [], "version_ids": [] }`。轮次按新到旧排列，仅省略各轮的 `points`。`version_ids` 为该修订已关联的登记版本。 |
| POST | 基址 | `{"revision_no":1}` → `201`，保存并返回第 1 轮 `draft`。 |
| GET | `/{analysisId}` | `200`，返回完整轮次。 |
| POST | `/{analysisId}/review` | `{"action":"submit"}`、`{"action":"return","feedback":"明确停用账号的提示"}` 或 `{"action":"approve","version_id":"version-1"}` → `200`，返回更新后的轮次。审批时 `version_id` 可省略，提供时须已关联到源修订。 |
| POST | `/{analysisId}/optimize` | 无请求体。使用源修订、退回轮次的功能点和反馈生成下一轮 → `201`，返回新 `draft`。 |

轮次示例

```json
{
  "id": "analysis-2",
  "requirement_id": "req-1",
  "project_id": "project-1",
  "revision_no": 1,
  "round_no": 2,
  "parent_analysis_id": "analysis-1",
  "status": "approved",
  "points": [
    {
      "title": "停用账号登录",
      "description": "登录服务禁止已停用的账号建立会话。",
      "acceptance_criteria": ["使用停用账号登录时，页面显示账号已停用提示。", "系统不建立登录会话。"]
    }
  ],
  "version_id": "version-1",
  "created_at": "2026-11-23 13:10:00",
  "submitted_by": "editor",
  "submitted_at": "2026-11-23 13:11:00",
  "reviewed_by": "owner",
  "reviewed_at": "2026-11-23 13:12:00"
}
```

`parent_analysis_id`、`feedback`、`version_id`、`submitted_by`、`submitted_at`、`reviewed_by` 和 `reviewed_at` 为空时省略。`draft` 可提交为 `submitted`，`submitted` 可退回为 `returned` 或批准为 `approved`。退回须提供去空格后 1–2000 字符的反馈。每个 `returned` 轮次可生成一个后继草稿，重复该流程直至批准。已批准轮次保持固定，新修订开启独立流程。生成、优化、提交、退回和批准成功后记录工作区审计事件。

模型使用租户设置的 OpenAI 兼容连接。生成和优化为同步请求，最多调用模型一次，超时取租户预算与 90 秒的最小值，输出预算取租户预算与 8192 tokens 的最小值。源标题与正文加起来最多 32 KiB。模型结果 JSON 最多 32 KiB，允许 1–20 个功能点，每条标题 1–160 字符、描述 1–1200 字符、1–8 条验收标准，每条标准 1–500 字符。模型调用或校验失败时保持现有轮次，允许重试。

错误体为 `{"error":"说明"}`。`400` 为请求格式、修订号或反馈不合法，`401` 为会话无效，`403` 为角色不足，`404` 为当前租户找不到资源，`409` 为轮次重复或状态不允许，`413` 为 JSON 请求体超过 16 KiB，`422` 为模型设置、源长度、模型凭据或版本关联不符合要求，`500` 为内部错误，`502` 为模型调用或输出失败。

## 用例生成交接

工作台收到审批成功响应后发出 Vue `generate-cases` 事件。载荷为 `{ projectId, requirementId, analysisId, versionId? }`，`versionId` 取审批记录中的 `version_id`。重新打开批准轮次时也可以通过「生成用例」再次发出事件。用例模块使用已批准轮次的 `revision_no` 和 `points`，并校验租户、项目、需求和可选版本。Go 侧可使用 `(*store.Store).GetRequirementPointAnalysis(ctx, tenantID, requirementID, analysisID)` 读取轮次。

集成注册函数为 `(*api.Server).registerRequirementPoints(mux *http.ServeMux)`。SQLite 启动阶段在 `store.MigrateRequirements(db)` 后调用 `store.MigrateRequirementPoints(db)`。
