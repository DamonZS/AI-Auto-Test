# 本地 Playwright 录制

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

## Web 录制剩余差距

Web 页面要操作测试人员机器上的独立 Playwright 浏览器，需要本地录制服务来启动、停止、取消会话并把草稿交回控制面。届时再设计本机连接和会话管理、Web 草稿预览，以及如何接入现有审批流程。

## 验证

```powershell
python -m pytest -q tests/test_recorder.py tests/test_recorded_steps.py tests/test_case_cli.py
```

真实 Chromium 通过 loopback 表单验证导航、可访问名称、字段引用、输入及查询值不进入草稿。测试同时覆盖终端停止与取消、关闭标签页自动保存，以及会话结束后释放上下文。
