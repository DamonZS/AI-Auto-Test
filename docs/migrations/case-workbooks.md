# 桌面工作台 XLSX 用例

桌面窗口通过 Qt 文件对话框在本机读取和写入 `.xlsx`。导入只填充待编辑的表单。用户确认项目、运行环境、测试数据和步骤后，通过现有用例流程保存草稿并审核。

## 格式范围

- 结构化 UI 工作簿每份包含一个场景。导入沿用 CLI 的隐藏回放摘要和可见步骤一致性校验。导出保留现有场景 JSON。修改步骤后应从用例重新导出。
- 旧版自然语言工作簿读取第一张工作表。必填列为 `url` 和 `scenario`，可选列为 `name`。列别名和空行处理沿用 `auto_ai/excel_reader.py`。多行时逐条选取。旧版 URL 用于确认目标环境，运行采用项目或所绑定环境的地址。
- 旧版自然语言用例、Laya 17 列工作簿、`.xls`、接口和性能用例不在结构化 XLSX 往返范围内。
- 普通浏览器沿用现有 JSON 录制脚本导入。离线转换继续使用 `scripts/case_cli.py` 的 `import-xlsx` 和 `export-xlsx`。

## 集成约定

- 从 `platform/web/src/components/` 导入 `CaseWorkbookImport.vue`。`@imported` 接收 `{ name, scenario }`，`scenario` 是结构化对象。`@legacy-imported` 接收 `{ name, scenario, url }`，`scenario` 是自然语言文本。
- 对结构化 UI 场景渲染 `CaseWorkbookExport.vue` 并传入 `:name` 和 `:scenario`。`scenario` 接受场景对象或序列化后的 JSON 字符串。
- 原生安装包需要将 `packaging/desktop_workbook.py` 与 `desktop_window.py` 同级收录，并随包提供 `openpyxl`。`PySide6` 已提供 Qt WebChannel。
