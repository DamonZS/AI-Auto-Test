# MCP 客户端接入

## 签发令牌

在平台进入目标工作区。工作区所有者可在 MCP 页面签发、查看到期时间和撤销 MCP 令牌。签发请求固定使用 `scope: "mcp"`，复用 [`/api/auth/service-tokens`](auth.openapi.yaml) 的现有权限与租户校验。令牌有效期为 90 天。明文仅在签发成功时显示，页面仅将其保留在内存中。关闭明文、离开页面或刷新后无法找回。请将令牌保存到客户端的凭据配置中。

轮换时先签发新令牌，更新客户端并用 `tools/list` 验证连接，再撤销旧令牌。客户端更换凭据后须重新初始化会话。

## 配置连接

| 配置项 | 值 |
| --- | --- |
| 传输方式 | Streamable HTTP |
| URL | Go 控制面的 `/mcp`，如 `https://platform.example.com/mcp`。本地联调使用 Go 的监听地址。 |
| 鉴权 | `Authorization: Bearer <MCP 令牌>`。仅支持自定义头的客户端可使用 `X-Agent-Token: <MCP 令牌>`。选定一种后在会话内保持一致。 |
| 协议版本 | `2025-06-18` |

服务依据令牌确定租户。客户端配置字段名和凭据注入方式以客户端文档为准。Go 同时托管 Web 时，可以直接使用页面显示的地址。Vite 开发服务目前只代理 `/api`，开发时请将 MCP 客户端指向 Go 服务。

1. 向 `/mcp` 发送 `initialize`。POST 带 `Content-Type: application/json` 和 `Accept: application/json, text/event-stream`。从响应头保存 `Mcp-Session-Id`。
2. 发送 `notifications/initialized`。本次会话的后续 POST 带相同鉴权、`Mcp-Session-Id` 和 `MCP-Protocol-Version: 2025-06-18`。
3. 用 `tools/list` 验证连接并查看工具参数。`401` 表示凭据无效、过期或已撤销。`403` 时检查令牌范围及 `Origin` 与 `Host` 是否一致。

## 可用工具

- 用 `list_cases` 查用例，用 `list_bugs` 查缺陷。
- 用 `create_test_task` 按已有用例 ID 排队，用 `retest` 按已有缺陷关联的用例复测。用 `get_run_status` 和 `get_run_report` 查询返回的 `run_id`。
- 用 `generate_test_cases_from_code` 生成普通功能测试草稿。调用前先确认客户端支持 Sampling。生成结果须在平台审核定稿后才可执行。执行使用工作区内已有配置及用例。

## 启用 Sampling

在客户端启用 Sampling 并确认客户端会在 `initialize` 发送 `params.capabilities.sampling: {}`，能读取原工具 POST 上的 SSE，并能通过同一会话的另一条 POST 回传 `sampling/createMessage` 的响应。由客户端处理模型和用户确认。缺少 Sampling 能力时，普通 JSON 工具仍可使用。只有客户端实际声明该能力并能处理服务端请求时，才能使用源码生成工具。

消息格式、超时、草稿范围及反代要求见 [MCP Sampling 协议与验收](mcp-sampling.md)。
