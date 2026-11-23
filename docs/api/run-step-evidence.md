# 结构化 UI 回放证据

执行端在现有 `POST /api/runs/{id}/finish` 请求中可以附带 `steps` 数组。每条记录包含从 1 开始的 `step`、步骤 `action`、`passed` 或 `failed` 的 `status` 和非负 `duration_ms`。失败步骤终止本次回放，未执行的步骤无记录。

结构化 UI 场景中，某一步显式设置布尔值 `"screenshot": true` 时，worker 在该步结束后尝试采集视口截图，通过步骤证据的 `screenshot_jpeg` 字段传输 Base64 编码的 JPEG。选中的失败步骤也可取证。每次执行最多尝试 4 次，每次限时 2 秒，每张原始 JPEG 不超过 128 KiB，最大尺寸为 1280 × 720。采集失败、图像文件过大或超出次数上限时，该步只附带 `screenshot_error`，取值为 `unavailable`、`too_large` 或 `limit`，步骤判定照常。

截图遮罩主页面和 iframe 中的 `input`、`textarea`、`select` 及 `contenteditable` 元素。页面将值另行渲染到普通文本、自定义控件或 Canvas 时，这些内容仍可能出现在截图中。此类页面仅对确认适合留存的步骤开启截图。

Go 在结束执行的同一事务中，按租户和运行 ID 将截图及其他步骤证据写入现有 `run_step_evidence.steps_json`。`GET /api/runs/{id}` 返回该数组，没有逐步证据的历史执行返回空数组。工作台在执行详情的步骤表按行展示截图和采集状态。执行列表仍只返回摘要。
