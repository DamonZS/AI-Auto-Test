# 从需求生成测试草稿

- 路由通过 `s.registerCaseGenerationRoutes(mux)` 接入。在 `store.Open` 的需求迁移之后调用 `store.MigrateCaseGeneration(db)`。
- `CasesView` 接收 `projectId` 和 `generationTarget`。需求页传来的 `analysisId` 对应 `POST /api/cases/generate` 的 `review_id`。改选需求后按新需求生成。`generated` 事件携带完整成功响应，用例列表据此刷新。
- `review_id` 只接受同租户、同项目、同需求的已批准功能点评审。指定后使用该轮的原始需求修订与功能点。若传入 `version_id`，该修订须关联此版本。省略 `review_id` 时使用所选版本关联的最新修订，或需求的最新修订。
- `case_generation_sources` 按 `tenant_id` 和 `case_id` 保存 `requirement_id`、不可变的 `revision_no` 和可选的 `version_id`、`review_id`。可通过 `GET /api/requirements/{id}/history` 查询对应的需求快照。
- 需求应填写可发送给租户配置模型的产品行为描述。输入检查会拒绝代码块及常见的显式凭据格式，需求提交者负责移除其他敏感内容。模型密钥只用于上游请求的认证头。

| 类型 | 场景结构与执行方式 |
| --- | --- |
| `functional` | `{"steps":[{"action":"打开项目列表","expected":"显示项目列表"}]}`。每条草稿含 1–15 条有序步骤，步骤和预期各 1–500 字。定稿后按步骤人工验证。 |
| `ui` | 沿用 UI 工作器的 `steps`。模型输出 `assert_text` 或 `wait_text`，平台写入项目首页的 `navigate` 步骤。 |
| `api` | 沿用 API 工作器的 `request` 和 `assertions`。仅生成项目内相对路径、`GET` 或 `HEAD` 和一条 2xx 断言。 |
| `perf` | 沿用性能工作器的 `request`、`concurrency`、`duration_seconds`、`max_requests`、`timeout`。AI 草稿使用项目内 `GET` 或 `HEAD`，限 5 并发、30 秒、100 次请求及单次 10 秒超时。 |
| `security` | `{"checklist":["authentication","sensitive_data","configuration"]}`。平台提供固定文案供人工对照文档，允许选择其中任意非空子集。内容涵盖认证与权限要求、敏感数据处理和可配置项。 |

定稿后的 `ui`、`api`、`perf` 可执行和安排计划。`functional`、`security` 走人工流程。详见 [用例审核](case-review.md)。
