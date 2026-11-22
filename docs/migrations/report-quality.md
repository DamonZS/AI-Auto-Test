# Lemon 测试报告校验

`agents.report_quality.check_report_completeness(report)` 按固定顺序检查结构化报告，无需调用模型。

| 字段 | 完整度规则 |
| --- | --- |
| `test_scope_environment` | 非空文本，描述测试范围和环境 |
| `case_statistics` | `total`、`executed`、`passed`、`failed`、`blocked` 均为非负整数。通过、失败、阻塞数之和等于执行数，执行数不超过总数 |
| `requirement_coverage` | `covered`、`total` 均为非负整数，覆盖数不超过总数 |
| `execution_rate`、`pass_rate` | 取值在 `[0, 1]` 的数值。校验取值范围，保留上游的统计与舍入口径 |
| `defect_distribution` | 按优先级提供非负整数计数，空映射表示无缺陷 |
| `residual_risks`、`conclusion` | 非空文本，无残余风险时可填「无」 |
| `review_signoff` | `status` 为 `not_required`、`pending`、`approved` 或 `rejected`。`approved` 还须有非空 `reviewer` |

报告风险等级为 P0/P1、缺陷分布中存在 P0/P1，或结论包含「上线」时，必须人工审核。优先级匹配忽略大小写及首尾空白。「建议上线」「有条件上线」「暂缓上线」均触发审核。

字段缺失或不合法时，返回 `incomplete`，并在 `missing_sections` 中列出对应字段。字段齐全但仍需审核时，返回 `review_required`。其他情况返回 `complete`。`requires_human_review` 表示是否命中审核规则，批准后仍为真。签核人身份及签核记录由调用方提供。
