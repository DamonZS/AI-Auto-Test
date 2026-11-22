# 租户认证上线与验收

## 入口与权限

- Web 使用 `POST /api/auth/login` 获取 8 小时 Bearer 会话。`GET /api/tenants` 返回当前账号的 membership。租户业务请求通过 `X-Tenant-Id` 选择其中一个租户，服务端逐次检查角色。`POST /api/auth/logout` 撤销当前会话。浏览器刷新后重新登录。
- `viewer` 读取本租户的 Web 资源。`editor` 还可编辑业务数据、运行测试和管理调度。`owner` 还可修改租户设置、裁决自愈补丁和管理服务令牌。每个已配置的账号可创建独立工作区，并成为该工作区的 owner。
- `worker` 令牌只用于 `/api/agent/settings`、`/api/runs/heartbeat`、`/api/runs/{id}/heartbeat`、`/api/runs/{id}/finish` 和 `/api/test-data/datasets/{id}/raw-records`。`mcp` 令牌只用于 `/mcp`。两者接受 `Authorization: Bearer …` 或 `X-Agent-Token`。传入的 `X-Tenant-Id` 不改变服务令牌绑定的租户。
- 无效凭据返回 401。有效凭据的 scope、角色或 membership 不足时返回 403；已授权租户内查询其他租户 ID 对应的资源，继续使用现有 404 语义。
- Go 租户处理器可调用 `authenticatedActor(r)` 取得已验证的用户名，用作审核记录的操作者。显式原型兼容模式下返回空字符串。
- owner 使用 `/api/auth/service-tokens` 签发和查看服务令牌、按 ID 撤销。明文仅在签发时返回，90 天到期。轮换时先签发新令牌，再更新客户端并验证，最后撤销旧令牌。站点通过 HTTPS 提供服务，入口对登录实施速率限制。

## 迁移步骤

1. 暂停当前服务与 worker，备份 SQLite 数据库及配套 `.key`。使用 SQLite 一致性备份，或停机并确认 WAL 已完成检查点后复制文件。先在备份副本试跑以下步骤。
2. 使用同一二进制和 `-db` 路径运行一次性账号配置。进程环境注入 `PLATFORM_AUTH_USERNAME` 和 `PLATFORM_AUTH_PASSWORD`，密码 12–1024 字节。现有 `.env` 文件的值由部署入口导出到进程环境，Go 二进制仍按现有方式读取进程环境。配置完移除该进程环境中的明文密码。

   ```powershell
   go run ./cmd/server -db <db> -provision-user -auth-tenant <existing-id> -auth-role owner
   ```

   新库加 `-auth-tenant-name <name>` 可在同一事务创建指定租户。给既有用户名添加或调整 membership 时，同一次命令会更新该账号密码并撤销其所有 Web 会话。可用同一命令为各历史租户明确配置不同的 owner/editor/viewer。
3. 新认证表通过 SQLite `CREATE TABLE IF NOT EXISTS` 增量创建。既有业务数据和历史 `users` 保留，历史角色不会自动成为登录权限。确认每个历史租户至少有一个实际 owner 后，按现有 `-addr`、`-db` 和 `-web` 参数启动服务器。
4. 让 owner 逐租户签发 `worker` 和 `mcp` scope 令牌。Python 配置入口为 `python -m agents.worker_credentials --server <url>`，通过 `PLATFORM_WORKER_TOKEN` 注入凭据，进程环境优先于仓库根目录 `.env`。MCP 客户端配置 `Authorization: Bearer …` 并指向 `/mcp`。
   worker 在每次领到任务后读取最新租户设置。接口、性能和结构化 UI 任务可在模型 key 为空时执行。自然语言 UI 需要模型 key，设置变更从下一次领到任务时生效。
5. 用旧库副本验业务行数、既有设置密钥解密和读写路径。如果需要回滚，暂停入口与 worker 并优先保留当前数据库备份以保住切换后的写入。需恢复备份时同步恢复 `.key`。旧版本使用原型认证，回滚期间保持外部入口关闭。

## 验收

- 正常登录、列出 membership、切换已获授权的租户、退出后旧会话失效。
- 验证三种角色、缺失/过期/撤销凭据，以及服务令牌 scope 与租户边界。worker 领取并回报本租户用例，MCP 正常列出工具。
- 用临时旧版 SQLite 库验证自动增量与显式配置，对照升级前后的业务数据。
- 从 `platform/server` 运行 `go test ./... -run 'TestAuth'` 验证生产认证行为。`go test ./...` 的测试进程为历史 fixture 启用原型身份。生产 `cmd/server` 始终显式使用正式认证配置。
