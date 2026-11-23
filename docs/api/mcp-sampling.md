# MCP Sampling 协议与验收

依据项目 `Agents.md` 第 7–10 节及 Sampling 进度记录。本轮使用现有 `POST /mcp`、租户认证、用例审核、修订历史与执行队列。协议版本为 `2025-06-18`。

令牌与 Streamable HTTP 客户端配置见 [MCP 客户端接入](mcp.md)。Sampling 由客户端声明和处理。

## HTTP 流程

1. 客户端用 `initialize` 声明 `params.capabilities.sampling: {}`，保存响应头 `Mcp-Session-Id`，随后发送 `notifications/initialized`。后续请求携带相同认证、会话头及 `MCP-Protocol-Version: 2025-06-18`。
2. 客户端以 `tools/call` 调用 `generate_test_cases_from_code`，请求头 `Accept` 包含 `application/json, text/event-stream`。提交 `project_id`、`language`、`code`、`requirements` 和 `test_types`。`code` 上限为 256 KiB。`project_id`、`language`、`requirements` 上限分别为 128、64、4096 字节。`test_types` 支持 `api`、`ui` 和涵盖两者的 `functional`。只提交该项目生成普通功能用例所需的源码，并在客户端确认是否允许发给所用模型。
3. 原 POST 返回 `text/event-stream`，首个事件为 `sampling/createMessage`，包含一次源码上下文及唯一 JSON-RPC ID。客户端通过另一条 POST 提交同 ID 的 `result`（`role: "assistant"`、`content: {"type":"text","text":"…"}`）或 JSON-RPC `error`。匹配的回包获得 `202` 和空响应体。错配的会话返回 `404`，错配或重复的消息 ID 返回 `400`。
4. 原 SSE 随后返回与原 `tools/call` ID 对应的最终结果并关闭。成功时 `structuredContent` 给出 `generated: true`、`status: "needs_review"`、`project_id` 和 `draft.cases`（用例 ID、名称、类型和草稿状态）。模型拒绝、超时或无效输出时返回 `isError: true`。

普通工具发现、查询和轮询继续返回 JSON，也支持既有无会话调用。未初始化 Sampling 会话、未声明能力或未接受 SSE 时，生成工具返回 `isError: true`。首版没有任务包、平台模型回退、独立 GET 流或重连重放。`GET /mcp` 可返回 `405`。会话绑定路由认证得到的租户以及 `Authorization` 和 `X-Agent-Token` 的凭据摘要。会话仅在当前服务进程有效，初始化后 30 分钟过期，单会话同时最多一个 Sampling 等待，单次最多等待 90 秒。全进程最多 1024 个会话，容量用尽时初始化返回 `503`。

## 草稿与执行

模型只负责提出普通功能用例。首版接受 API 的 GET/HEAD、项目内普通相对路径及成功状态码断言，或 UI 的可见文本断言（执行前自动打开项目首页）。每批最多 10 条。固定场景字段映射到已有执行器，不接收任意脚本、请求体、请求头、查询参数、外部目标或安全测试步骤。

服务端以同一事务创建 `draft` 用例、初始 `case_revisions` 和审计记录，标记 `generation_mode=sampling` 及 `expected_source=code_inference`。只保存经校验和重建的场景。原始源码和模型回包均不落盘，也不进最终结果、错误、审计或后续修复提示。平台拒绝草稿字段中原样回显的源码，人工审核须核验场景和需求。在 Web 通过原有审核后，MCP 创建任务和复测均要求 Sampling 用例状态为 `approved`。普通草稿继续沿用原有 MCP 入队契约。

`get_run_report` 保留现有报告。对失败、错误或待复核的执行附带定性的 `repair_hint`，提醒调用方核对已有证据和已审核用例，修复后再复测。

## 验收

- 使用真实 `httptest.Server` 发起原 SSE POST 和独立回包 POST，验证 `202`、正确关联、最终事件与流关闭。
- 错租户、错凭据、错 ID 和重复回包不得推进原 SSE。未声明 Sampling 时不得产生草稿或向模型发送源码。
- 草稿可在现有审核与修订接口读取。MCP 在定稿前拒绝排队，定稿后执行端可领取场景并回报，既有工具可轮询及查询报告。
- 正常结果、拒绝结果、存储与审计均不得出现提交的源码或原始模型回包。跨项目及不受支持的场景不得入库。

## 主控接入

- `POST /mcp` 使用 `mcp` scope 令牌。服务端按令牌确定租户，更换凭据后重新初始化会话。
- Web 和 MCP 执行入口均检查 Sampling 用例状态，只有 `approved` 的 Sampling 用例可入队。
- 浏览器发送 `Origin` 时，其主机必须匹配 `Host`，否则返回 `403`。反代需校验公开主机名，透传 `Host`、`Mcp-Session-Id`、`MCP-Protocol-Version` 并流式转发 SSE。多实例部署需要同一会话的初始化、工具调用与回包命中同一进程。
