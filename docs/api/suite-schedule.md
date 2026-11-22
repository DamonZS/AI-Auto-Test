# 套件每周调度

`/api/suites/{id}/schedule` 沿用单用例调度的 JSON 格式和 `X-Tenant-Id`。套件必须属于请求租户。

| 方法 | 请求 | `200` 响应 |
| --- | --- | --- |
| `GET` | 无 | `{"schedule": null}` 或 `{"schedule": {…}}` |
| `PUT` | `{"enabled":true,"weekday":1,"hour":4,"minute":0}` | 保存后的调度对象 |
| `DELETE` | 无 | `{"suite_id":"…"}`，可重复删除 |

调度对象包含 `suite_id`、`enabled`、`weekday`、`hour`、`minute` 和 `next_run_at`。`weekday` 为 0（周日）至 6（周六），`hour` 为 0–23，`minute` 为 0–59。启用时 `next_run_at` 为服务端本地时区的 RFC 3339 时间，停用时为空字符串。无效 JSON、超出范围或缺少租户头返回 `400`，租户或所属套件不存在返回 `404`，存储错误返回 `500`。

启用后，在匹配的本地分钟读取套件当前的 `case_ids`。成员须非空且全部属于套件租户并处于 `approved`。满足条件时在同一事务中创建一条 `suite_runs` 和按成员顺序排队的子 `runs`，保留用例的数据绑定。任一成员不满足条件则跳过整个批次。每个租户的每个套件在同一本地分钟最多入队一次，即使调度被重新保存或删除后重建。错过的分钟不补跑。
