# 双形态部署规格与验收

基线为 `a3a65d4`。交付范围是 `deploy/`、`packaging/`、`.github/workflows/` 和 `docs/deployment/`。Go 提供控制面和同源 Vue 静态站点，Python 使用现有 `agents.ui_runner` 领取并回报任务。

## 交付规格

- 服务端以 Linux 容器发布。Compose 管理 Go/Web 与 Python worker，共享服务内网。HTTP 默认只发布到宿主机回环地址，控制面状态使用持久卷。
- 桌面最小交付目标是 Windows x64 每用户 Inno Setup EXE 与 macOS arm64 拖放式 DMG。安装包自带 Go、已构建 Vue、Python worker 运行时以及 Playwright Chromium。用户启动与停止均有入口，Web 监听回环，Windows 后台进程不弹控制台。
- 新安装的数据、SQLite 主密钥、缓存及日志写入用户数据目录。构建只带显式选定的源码、产物与第三方运行时。仓库 `.env`、数据库、密钥和真实样本不进入安装包或镜像。
- 两种部署都默认使用正式认证。管理员用同一个 Go 二进制预配 owner，Web 使用登录会话，Python 使用单独签发的 worker scope token。桌面从启动环境或用户数据目录读取 token，再注入 worker 环境。安装和构建无需凭据。
- macOS 在原生 runner 上构建及验证。首版产物用于内部验收，正式外发需补签名与 macOS 公证。

## 验收步骤

1. 使用默认发布入口启动 Web 与控制面。验证首页和静态资源有效，未登录的 `/api/tenants` 返回 401，已登录的 owner 只能看到预配的验收租户。
2. 从空库临时预配合成账号与租户，通过正式登录创建项目并签发 worker 服务令牌。提交只访问本机控制面的结构化 UI 测试，确认真实 Chromium 断言通过，且 Python worker 经 heartbeat 领取并回报 `passed`。测试结束停止进程并清理测试数据。
3. Windows 实跑静默安装、启动、停止及卸载。验证程序与数据目录分离，安装源无本地凭据、数据库或样本。macOS runner 生成、挂载 DMG，复制 `.app` 到测试安装目录，使用该安装副本执行相同测试。
4. Linux runner 实跑 Compose 构建、空库启动、相同 worker 用例及持久化复验。各平台通过后分别上传构建产物。本地没有可用 Docker 或 Mac 主机时，交付脚本和原生 CI，并记录具体未验证项。

## 实现顺序

1. 编写共享 smoke 与构建输入边界，验证空库、新建任务和回报的真实接口。
2. 实现桌面安装构建与启动生命周期，优先实跑 Windows。
3. 实现容器发布入口和原生三平台 CI，复验产物与操作文档。
