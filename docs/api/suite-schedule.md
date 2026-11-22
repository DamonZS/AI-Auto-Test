# 套件运行与每周调度

## 并发上限

套件默认 `max_concurrency: 1`。`GET /api/suites` 和 `POST /api/suites` 的套件对象包含该字段。使用 `PATCH /api/suites/{id}` 保存设置，请求为 `{"max_concurrency":2}`，成功返回 `200` 和 `{"id":"…","max_concurrency":2}`。上限必须为正整数，缺省或无效值返回 `400`，套件不存在或不属于请求租户返回 `404`，存储错误返回 `500`。沿用 `X-Tenant-Id`。

手动 `POST /api/suites/{id}/run` 和计划触发都把套件当前的 `max_concurrency` 快照保存到批次。手动入队响应和 `GET /api/suite-runs/{id}` 返回批次快照。编辑套件只影响之后创建的批次。旧套件和旧批次迁移后上限为 `1`。

worker 从当前租户最早可领取的执行中领取。运行中的子执行占用所属批次的并发名额，批次满额时跳过其余待执行成员。终态释放名额。批次按成员顺序领取并展示，多个 worker 的完成顺序可能不同。单个 worker 仍串行执行。

## 每周调度

`/api/suites/{id}/schedule` 沿用单用例调度的 JSON 格式和 `X-Tenant-Id`。套件必须属于请求租户。

| 方法 | 请求 | `200` 响应 |
| --- | --- | --- |
| `GET` | 无 | `{"schedule": null}` 或 `{"schedule": {…}}` |
| `PUT` | `{"enabled":true,"weekday":1,"hour":4,"minute":0}` | 保存后的调度对象 |
| `DELETE` | 无 | `{"suite_id":"…"}`，可重复删除 |

调度对象包含 `suite_id`、`enabled`、`weekday`、`hour`、`minute` 和 `next_run_at`。`weekday` 为 0（周日）至 6（周六），`hour` 为 0–23，`minute` 为 0–59。启用时 `next_run_at` 为服务端本地时区的 RFC 3339 时间，停用时为空字符串。无效 JSON、超出范围或缺少租户头返回 `400`，租户或所属套件不存在返回 `404`，存储错误返回 `500`。

启用后，在匹配的本地分钟读取套件当前的 `case_ids` 和 `max_concurrency`。成员须非空且全部属于套件租户并处于 `approved`。满足条件时在同一事务中创建一条 `suite_runs` 和按成员顺序排队的子 `runs`，保留用例的数据绑定。任一成员不满足条件则跳过整个批次。每个租户的每个套件在同一本地分钟最多入队一次，即使调度被重新保存或删除后重建。错过的分钟不补跑。
