# 浏览器与增强档

## Playwright

工作区 owner 在「设置 → 浏览器」中选择浏览器并保存。每次领到 UI 任务时，正式 worker (`python -m agents.worker_credentials`) 读取该租户的设置。选项对结构化回放和自然语言 UI 用例都生效。结构化回放和浏览器选项可以在模型名称、Base URL 与 API Key 均留空时使用。

| 选项 | 执行机要求 |
| --- | --- |
| 执行机默认 | 使用 `PLATFORM_BROWSER`，变量未设置时使用 Chromium |
| Chromium、Firefox、WebKit | 在 worker 的 Python 环境用 `python -m playwright install <浏览器>` 安装。参数可用 `chromium`、`firefox` 或 `webkit`。Linux 容器还需相应系统库 |
| Google Chrome、Microsoft Edge | 在执行机安装对应浏览器。Playwright 通过 `chrome` 或 `msedge` 通道启动 |

桌面和容器发行包默认自带 Chromium。所选浏览器不可用时，运行报告返回启动错误。已保存的浏览器偏好不会因执行机变更而自动修改。

`GET /api/settings` 和 `GET /api/agent/settings` 返回 `playwright_browser`。`PUT /api/settings` 接受 `chromium`、`firefox`、`webkit`、`chrome`、`msedge` 或空字符串。空字符串恢复执行机默认。旧客户端省略字段时保留当前选择。旧数据库自动新增此字段，默认使用执行机配置。

## 增强档

- **Ego** 本机冒烟已用 Windows 上的 `ego-browser` v2.2.1 验证。带桌面的机器可从源码目录运行 `ego-browser skill` 查看本机契约，再运行 `python -m agents.ego_smoke`。它在新任务空间内打开合成表单，验证输入、点击和渲染结果，结束时释放任务空间。租户任务仍由 Playwright 执行。启用 Ego 请求会在执行记录中显示降级原因。
- **Laya** 当前适用于自然语言 UI 用例。工作区模型配置须指向兼容现有 Laya 候选与概率协议的 HTTPS 服务。结构化回放按其步骤和断言执行。运行时没有已验证的 Laya 候选概率时，按现有决策逻辑处理。
- **JEV** 缺少已验证的服务地址与决策契约。启用 JEV 请求会在执行记录中显示原因，继续使用原有决策流程。
