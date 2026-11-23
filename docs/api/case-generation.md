# 从需求生成测试草稿

- 路由通过 `s.registerCaseGenerationRoutes(mux)` 接入。在 `store.Open` 的需求迁移之后调用 `store.MigrateCaseGeneration(db)`。
- 页面使用 `CaseGenerationPanel`，传入 `projectId` 和 `requirements`。需求条目沿用需求列表的 `id`、`project_id`、`code`、`title` 字段。可用 `initialRequirementId` 和 `initialVersionId` 承接需求页的预选。用户改选需求时清除预选版本。
- `generated` 事件携带 `POST /api/cases/generate` 的完整成功响应。用例列表据此刷新，沿用现有人工审批入口。
- `case_generation_sources` 按 `tenant_id` 和 `case_id` 保存 `requirement_id`、不可变的 `revision_no` 及可选 `version_id`。用同租户的 `GET /api/requirements/{id}/history` 查询该编号对应的原始快照。
- UI 草稿从项目首页开始，仅使用可见文本等待或断言。接口草稿使用项目内相对路径、`GET` 或 `HEAD` 及一条成功状态码断言。生成调用只写 `draft` 状态，用例获批后才可进入执行队列。
- 需求应填写可发送给租户配置模型的产品行为描述。输入检查会拒绝代码块及常见的显式凭据格式，需求提交者负责移除其他敏感内容。模型密钥只用于上游请求的认证头。
