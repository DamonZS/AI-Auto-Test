# 用例审核与 AI 修订

在用例页按项目、名称、类型或审核状态筛选。通过「新建用例」填写草稿，或使用「AI 生成」。编辑、查看和审核在独立面板内操作。

| 操作 | 请求 | 状态与修订 |
| --- | --- | --- |
| 提交审核 | 向 `POST /api/cases/{id}/review` 发送 `{"decision":"submit_review"}` | `draft` 进入 `in_review`。退回版本须先修订。 |
| 退回 | 同一路由发送 `{"decision":"reject","feedback":"补充空列表的预期"}` | `in_review` 回到 `draft`。记录反馈、反馈对应的版本和会话审核人，内容版本保持不变。 |
| AI 修订 | `POST /api/cases/{id}/revise-with-ai` | 根据当前草稿、退回反馈与原始生成来源生成一条修订。成功后以 `draft` 保存下一内容版本，供人工核对、再次提交。 |
| 定稿 | 审核路由发送 `{"decision":"approve"}` | `in_review` 进入 `approved`。审核人来自 Web 会话。 |

操作使用租户 Web 会话和 `X-Tenant-Id`，需要编辑权限。反馈须为 1–2000 字。`review_feedback_version` 记录反馈针对的内容版本。审核操作写入审计日志，内容修订可通过 `GET /api/cases/{id}/history` 查看。模型输出不合格、未修改内容或请求期间工作流发生变更时，原草稿和反馈可继续查看。

`api.go` 中 `POST /api/cases/{id}/review` 使用 `s.tenant(s.reviewCaseWorkflow)`，`POST /api/cases/{id}/revise-with-ai` 使用 `s.tenant(s.reviseCaseDraft)`。`cases` 需要 `review_feedback TEXT NOT NULL DEFAULT ''` 和 `review_feedback_version INTEGER NOT NULL DEFAULT 0`。`case_generation_sources` 需要 `review_id TEXT NOT NULL DEFAULT ''`。

已定稿的 `functional` 和 `security` 用例详情提供「登记人工结果」。人工选择通过或失败，填写记录，按需关联当前用例项目的版本。表单调用 `POST /api/cases/{id}/manual-result`，保存后用返回的 `run_id` 打开执行记录。运行记录直接保存为最终状态。
