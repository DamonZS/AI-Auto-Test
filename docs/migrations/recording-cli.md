# 本地 Playwright 录制

## 桌面录制

1. 在 Auto-Test 桌面窗口的工具栏点击「录制用例」，输入 HTTP(S) 测试页面地址，点击「开始录制」。
2. 在独立的 Playwright Chromium 窗口操作页面。返回录制对话框，点击「停止并导出」并选择 JSON 文件保存位置。取消保存后，草稿留在对话框内，可再次选择「导出 JSON」。手动关闭录制标签页或浏览器后，也可返回对话框导出。
3. 在工作台「用例」页选择项目和 UI 类型，通过「导入录制脚本」打开草稿，复核步骤，补充断言和测试数据后保存。提交审核并通过后，点击「回放」查看执行状态、逐步耗时和报告。

点击「放弃」、关闭录制对话框或退出 Auto-Test 会丢弃未导出的草稿并关闭录制浏览器。关闭桌面主窗口收起到托盘时，录制继续运行。桌面安装包随附有界面的 Chromium 和供回放使用的无头组件。

## 命令行录制

在项目根目录运行。未安装 Chromium 时，先用项目环境执行 `python -m playwright install chromium`。

```powershell
python scripts/case_cli.py record-ui http://127.0.0.1:8080/ draft.json
```

CLI 在独立的 Chromium 上下文中打开测试页面。完成操作后回到终端输入 `stop` 导出草稿。关闭录制标签页也会保存。`quit`、stdin 到达 EOF 或 `Ctrl+C` 会放弃草稿。输出参数写 `-` 时，草稿 JSON 写到 stdout，操作提示写到 stderr。

草稿使用现有 [结构化 UI 场景契约](structured-ui-scenario.md)，只导出 `steps`，留待人工复核、绑定测试数据和补充断言。CLI 本身只写本地 JSON。

- 导航保存首个及后续主页面 HTTP(S) 地址的协议、主机和路径，省略查询参数和片段。点击引起的导航也会保留为单独一步，复核时可移除重复导航。
- 点击使用常见控件的角色和名称。名称来自 `aria-labelledby`、`aria-label`、关联的 `<label>` 或控件文本。无法得到名称的动作跳过。重复名称及复杂控件需要人工核对。
- 文本类控件的输入只生成 `fill` 和 `data_key`。字段键依次取 `data-key`、HTML `name`、`id`、可访问名称。连续编辑同一控件只记一步。录制器不读取控件输入值或按键内容。请使用测试数据，并核对页面是否把输入回显到了控件名或 URL 路径上。
- 当前覆盖一个标签页的主文档，不录制 iframe、弹窗交互、原生无角色的密码框、文件上传、下拉选择或断言。

浏览器版工作台通过导入 JSON 接收草稿。录制会话由桌面应用或 CLI 在本机操作。

## 验证

```powershell
python -m pytest -q tests/test_recorder.py tests/test_recorded_steps.py tests/test_case_cli.py
```

真实 Chromium 通过 loopback 表单验证导航、可访问名称、字段引用、输入及查询值不进入草稿。测试同时覆盖终端停止与取消、关闭标签页自动保存，以及会话结束后释放上下文。

在有 PySide6 和图形桌面的项目测试环境中，设置 `AUTOTEST_RECORDER_HEADED_TEST=1` 后运行相同命令，可验证 Qt 入口、真实有界面 Chromium、导出、放弃和退出清理。
