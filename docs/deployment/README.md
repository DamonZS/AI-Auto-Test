# Auto-Test 部署

构建依赖正式认证实现及 `agents/worker_credentials.py`。推送到 `main` 或手动运行发布流水线后，Linux、Windows x64 与 macOS arm64 runner 运行安装或启动验收，再上传各平台产物。现有数据库迁移见 [租户认证说明](../api/auth-migration.md)。

推送 `vX.Y.Z` 标签后，三平台全部通过才会发布到 [GitHub Releases](https://github.com/DamonZS/Auto-Test/releases)。构建会一并发布安装包和 `SHA256SUMS.txt`。如提交中带有 `docs/releases/vX.Y.Z.md`，发布页使用该说明，否则使用 GitHub 生成的变更摘要。

## Linux 服务端

需要 Docker Engine 和 Docker Compose v2。从仓库根目录运行。也可以从 CI 下载 `Auto-Test-linux-amd64.tar.gz`，以 `docker load -i` 导入镜像，然后设置 `AUTOTEST_IMAGE=autotest-platform:<构建提交>` 并跳过 `build`。

```bash
docker compose -f deploy/compose.yaml build control
```

首次部署先预配 owner。以下凭据仅进入一次性 Go 进程环境。新库用 `-auth-tenant` 指定唯一租户 ID，并用 `-auth-tenant-name` 指定新租户名称。旧库将 `-auth-tenant` 改为现有 ID，省略 `-auth-tenant-name` 并指定 `-auth-role owner`。更改旧库前停服务并备份数据库与对应的 `.key`。

```bash
read -r -p 'Owner username: ' PLATFORM_AUTH_USERNAME
read -rs -p 'Owner password: ' PLATFORM_AUTH_PASSWORD; printf '\n'
export PLATFORM_AUTH_USERNAME PLATFORM_AUTH_PASSWORD
docker compose -f deploy/compose.yaml run --rm -e PLATFORM_AUTH_USERNAME -e PLATFORM_AUTH_PASSWORD control \
  autotest-server -db /state/platform.db -provision-user -auth-tenant my-workspace -auth-tenant-name 'My workspace'
unset PLATFORM_AUTH_USERNAME PLATFORM_AUTH_PASSWORD
docker compose -f deploy/compose.yaml up -d --no-build --wait control
```

在本机打开 `http://127.0.0.1:8090` 登录。根据测试类型在租户设置页配置模型。owner 使用 `POST /api/auth/login` 取得 Web Bearer，再用 `POST /api/auth/service-tokens`，请求头带 `Authorization: Bearer …` 和 `X-Tenant-Id`，请求体为 `{"scope":"worker"}`。从签发响应读取一次性显示的 `token`。Worker 容器经内部 DNS 连接 `http://control:8090`，被测服务也必须能从容器访问。

```bash
read -rs -p 'Worker token: ' PLATFORM_WORKER_TOKEN; printf '\n'
export PLATFORM_WORKER_TOKEN
docker compose -f deploy/compose.yaml up -d --no-build worker
unset PLATFORM_WORKER_TOKEN
```

更新后使用同一 Compose 项目名与 `control-data` 卷。重新注入当前 worker token 后，以 `up -d --no-build --force-recreate worker` 完成轮换。`docker compose -f deploy/compose.yaml --profile worker down` 停止服务并保留卷。对外提供服务时由 HTTPS 反向代理接入回环端口，并按认证文档限制登录频率。

## Windows x64

构建机需要 Go 1.26、Node 22、Python 3.13 和经过校验的 Inno Setup。Windows CI 校验 Inno 6.7.3 安装程序 SHA-256 和 Authenticode。

```powershell
py -3.13 packaging/build_desktop.py --output artifacts --iscc 'C:\path\to\ISCC.exe'
```

运行 `Auto-Test-windows-x64-setup.exe` 后，从开始菜单打开 `Auto-Test`。在桌面窗口输入工作区名称、用户名和密码，创建本机首个工作区。注册成功会自动进入工作台。已初始化的数据库直接显示登录页。

关闭窗口会收起到通知区域。点击托盘图标恢复窗口，使用托盘菜单的「退出 Auto-Test」停止本机服务和 worker。开始菜单中的 `Stop Auto-Test` 会显示停止结果。卸载保留 `%LOCALAPPDATA%\Auto-Test` 下的用户数据。

在工作台签发 worker 令牌后，可将其放在 `%LOCALAPPDATA%\Auto-Test\worker-token.txt` 单行文件。仅当前用户应有读写权限。桌面检测到文件后启动 worker。轮换时退出桌面、替换令牌、重新启动，再撤销旧令牌。

## macOS arm64

在 Apple Silicon 和同版本构建工具上运行 `python3.13 packaging/build_desktop.py --output artifacts`。打开 `Auto-Test-macos-arm64.dmg`，把 `Auto-Test.app` 拖到 `Applications`。安装包当前用于内部验收。外发前需要签名与公证。

从 `Applications` 打开 `Auto-Test`，在桌面窗口创建工作区并注册管理员。应用图标出现在 Dock，菜单栏提供恢复窗口和退出入口。签发的 worker 令牌可写入 `~/Library/Application Support/Auto-Test/worker-token.txt`，文件权限设为 `600`。在启动器进程环境中也可直接提供 `PLATFORM_WORKER_TOKEN`。

## 验证与数据

- Playwright worker 默认使用 Chromium。可设置 `PLATFORM_BROWSER=chrome` 或 `PLATFORM_BROWSER=msedge` 使用执行机上已安装的对应浏览器，也可设置 `firefox` 或 `webkit`，并在同一 Python 环境安装对应的 Playwright 浏览器。使用多个执行进程时为每个进程设置所需浏览器。

- Windows 使用 PowerShell 7 执行 `./packaging/smoke_windows.ps1 -Installer artifacts/Auto-Test-windows-x64-setup.exe`。macOS 执行 `bash packaging/smoke_macos.sh artifacts/Auto-Test-macos-arm64.dmg`。Linux 执行 `bash deploy/smoke_compose.sh`。
- 冒烟使用临时账号、独立数据库和合成结构化 UI 用例，检查桌面注册表单、正式认证、Chromium 实际断言和 worker 回报。真实模型凭据只在运行时设置。
- 安装包自带 Python 和 Chromium。桌面监听 `127.0.0.1:8090`，数据库、主密钥、日志和截图位于用户数据目录。启动器通过 `PLATFORM_WORKER_CACHE_DIR` 将探索快照写入用户数据目录下的 `cache/`。Compose worker 将缓存写入 `worker-data` 卷的 `/state/cache`。
- 桌面日志在用户数据目录的 `desktop.log`。服务端通过 `docker compose -f deploy/compose.yaml --profile worker logs control worker` 查看日志。常规日志和报告仍需按业务数据治理要求存放。
