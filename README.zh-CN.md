# Auto-Test

[English](README.md) · 简体中文

Auto-Test 是一个软件测试工作台，用于从需求整理测试用例、审核并定稿、执行测试，以及追踪运行报告和缺陷。AI 可以辅助分析需求和生成用例草稿，测试结果由自动化断言或人工验证决定。

## 下载与安装

安装包和发布说明见 [GitHub Releases](https://github.com/DamonZS/Auto-Test/releases/latest)。

| 平台 | 下载 | 安装方式 |
| --- | --- | --- |
| Windows x64 | [安装程序 `.exe`](https://github.com/DamonZS/Auto-Test/releases/latest/download/Auto-Test-windows-x64-setup.exe) | 运行安装程序，再从开始菜单打开 Auto-Test。 |
| macOS Apple Silicon | [磁盘映像 `.dmg`](https://github.com/DamonZS/Auto-Test/releases/latest/download/Auto-Test-macos-arm64.dmg) | 打开映像，把 `Auto-Test.app` 拖入「应用程序」。 |
| Linux amd64 | [Docker 镜像归档 `.tar.gz`](https://github.com/DamonZS/Auto-Test/releases/latest/download/Auto-Test-linux-amd64.tar.gz) | 用 `docker load -i` 导入，再按部署指南使用 Docker Compose 启动。 |

下载后可对照 [SHA256SUMS.txt](https://github.com/DamonZS/Auto-Test/releases/latest/download/SHA256SUMS.txt) 校验文件。桌面首次启动时创建工作区和账户，再由工作区所有者在「设置 → 桌面 Worker」激活执行器。在设置中配置 OpenAI 兼容模型即可启用 AI 生成与定位恢复。Linux 初始化和凭据配置见 [部署指南](docs/deployment/README.md)。

## 从需求到测试结果

1. 创建项目和测试环境，录入需求。可以先评审需求的功能点，再从已批准的功能点生成用例草稿。
2. 提交用例审核。退回时填写反馈，修订后重新提交。审核通过并定稿后，再按用例类型执行自动化测试或登记人工结果。[用例审核流程](docs/api/case-review.md)
3. 运行后查看报告、步骤证据和失败断言。需要跟踪问题时登记缺陷，并关联用例用于复测。[运行报告](docs/api/run-report.md) · [缺陷管理](docs/api/bug-management.md)

| 用例类型 | 如何验证 |
| --- | --- |
| 手工功能 | 按有序步骤核对预期结果，人工登记通过或失败。 |
| UI 自动化 | 由 Playwright 执行浏览器操作和页面断言。 |
| API 自动化 | 由执行器发送配置的 HTTP 请求并检查响应断言。 |
| 接口性能 | 按配置的并发数、时长和请求数上限运行，查看延迟和错误指标。 |
| 防御性安全检查 | 人工按固定清单核对认证与权限、敏感数据处理和配置，登记通过或失败。 |

自动化执行和计划调度适用于已定稿的 UI、API 和性能用例。[用例类型与草稿生成范围](docs/api/case-generation.md)

桌面「用例」页可录制 UI 自动化步骤并导出草稿。结构化 UI 用例遇到定位漂移时，AI 可从当前作用域的同角色候选中提出新定位。通过原断言后生成候选补丁，工作区所有者批准后保存为新用例修订。[UI 定位自愈](docs/api/ui-healing.md)

## 本地开发与部署

- 控制面在 `platform/server`，Vue 前端在 `platform/web`。在 `platform/web` 安装依赖后运行 `npm run dev`，生产构建使用 `npm run build`。Vite 仅代理 `/api`，本地 MCP 客户端直接连接 Go 控制面。
- 从仓库根目录可用 `docker compose -f deploy/compose.yaml build control` 构建服务。首次部署先预配工作区所有者，再启动控制面并为 Worker 签发独立令牌。桌面构建、数据库迁移和升级步骤见 [部署指南](docs/deployment/README.md)。

## MCP 接入与审核边界

工作区所有者签发 MCP 令牌后，客户端可通过 Streamable HTTP 连接 `/mcp`，查询用例和缺陷、按已有用例排队测试或复测，并按运行编号查询状态和报告。支持 Sampling 的客户端也可以从源码生成普通功能测试草稿，草稿仍须在工作台审核定稿。审批和定稿在工作台完成。[MCP 客户端接入](docs/api/mcp.md) · [Sampling 协议](docs/api/mcp-sampling.md)

## 许可证与声明

项目许可证见 [MIT LICENSE](LICENSE)，版权和相关声明见 [COPYRIGHT.md](COPYRIGHT.md)。
