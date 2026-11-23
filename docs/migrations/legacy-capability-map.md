# 旧源码能力迁移映射

以当前 `platform/` 和 `agents/` 的可用入口为准。旧项目源文件保存在工作区外的本地归档，后续可据此续迁。

## `record-studio/`

| 能力 | 旧实现 | 主平台归属 | 状态 |
|---|---|---|---|
| UI/API/性能任务与执行记录 | `backend/app/runner.py` | `platform/server` + `agents/*_runner.py` | 已由主平台任务队列及 Go 到 Python worker 契约覆盖 |
| 用例步骤结构、编辑、确定性回放 | `backend/app/engine.py`, `runner.py`, `frontend/src/components/StepItem.vue` | `scenario.steps` + `agents/ui_runner.py` + `CasesView.vue` | 本次迁移 |
| 单用例每周调度 | `backend/app/scheduler.py`, `store.py` | `platform/server/internal/api/schedules.go` + 用例页计划入口 | 已迁移，Go handler 测试覆盖 |
| 套件成员、批量执行、历史汇总和每周调度 | `backend/app/runner.py`, `store.py`, `scheduler.py` | `platform/server/internal/api/suites.go`、`schedules.go`、`platform/web/src/views/SuitesView.vue` | 事务内保存有序成员和批次。工作台支持查询历史、汇总和维护每周计划。套件并发配置待迁移 |
| 录制事件转换 | `backend/app/engine.py`, `recorder_script.py` | `agents/recorded_steps.py`、`scripts/case_cli.py` | 可离线将导航、左键点击、填写转换成结构化步骤。填写仅保留测试数据字段引用 |
| 套件层级及可视化录制 | `backend/app/nodes.py`, `engine.py`, `recorder_script.py` | — | 节点继承、浏览器录制会话和操作入口待迁移 |
| 逐步骤执行结果、截图与诊断证据 | `backend/app/runner.py`, `shots.py`, `diag.py` | `runs` 报告与 Python runner 截图 | 失败报告持久化诊断类别与计数。逐步骤证据单独持久化待迁移 |

## `laya-pilot-main/`

| 能力 | 旧实现 | 主平台归属 | 状态 |
|---|---|---|---|
| Laya 候选决策服务 | `lib/model.mjs`, `worker.py` | `agents/laya_decision_provider.py`, `agents/ui_runner.py` | Python HTTP Provider 已接入；不迁移 Node demo server |
| Playwright 语义动作与主流程 | `lib/browser-provider.mjs`, `lib/engine.mjs` | `auto_ai/browser.py`, `agents/ui_runner.py` | 已有基线与 Laya 可选决策接入 |
| 页面 readiness | `lib/readiness.mjs` | `auto_ai/readiness.py`、`auto_ai/browser.py` | Playwright 导航后等待控件稳定，到达等待上限后返回快照。frame/shadow DOM 待迁移 |
| 行上下文和 Form 适配 | `dom.mjs`, `form.mjs` | — | 未迁移；基础页面快照和 `wait_text` 不是 Form 等价实现 |
| 浏览器网络/页面诊断与脱敏 | `lib/diagnostics.mjs` | `auto_ai/browser_diagnostics.py`、`agents/ui_runner.py` | 接入受控回放和失败报告，真实 Chromium 夹具验证请求失败、HTTP 错误与页面异常 |
| Excel 用例导入导出及回放校验 | `lib/workflow_excel.py`, `lib/workflow.mjs` | `agents/case_workbook.py`、`scripts/case_cli.py`、`packaging/desktop_workbook.py` | 校验结构化 UI 的隐藏回放摘要和可见步骤顺序，桌面桥接另支持旧版自然语言表。接入与格式范围见 [桌面工作簿](case-workbooks.md) |
| Laya HTTP 候选决策协议 | `lib/model.mjs`, `worker.py` | `agents/laya_decision_provider.py`, `agents/ui_runner.py` | 核心 API 协议已迁移；逐次概率、margin、缓存和推理轨迹尚未完整持久化 |
| Node CLI、演示服务、样例页面/模型权重 | `runner.mjs`, `run-demo-server.sh`, `examples/` | — | 本地演示和 Node 运行壳不迁移 |

## `lemon-TestPilot-main/`

| 能力 | 旧实现 | 主平台归属 | 状态 |
|---|---|---|---|
| 项目、需求、版本、用例、缺陷、执行和概览 | `src/workbench.tsx`, `workbench-api.mjs` | `platform/server/internal/api` + `platform/web/src/views` | 已有主平台 Go/Vue 实现，需以 Go API 测试与页面验证为覆盖证据 |
| 缺陷按签名复测及关闭后重开 | Node 工作台原型 | `runs.go`, `bugs.go` | 已有主平台实现，需以真实 API 测试为覆盖证据 |
| 七类 AI 入口、工作流顺序、必填及复核定义 | `workbench-api.mjs` | `agents/workbench_catalog.py` | 静态定义和人工复核门禁已迁移并测试；尚未接入 Deep Agents 模型执行 |
| 知识库分类和工作流引用规则 | `workbench-api.mjs`, `server.mjs` | `agents/workbench_catalog.py` | 分类标签定义已迁移；Chroma 租户知识检索未证明完成 |
| 报告章节和完整度检查规则 | `workbench-api.mjs`, `app.js` | `agents/report_quality.py`、`docs/migrations/report-quality.md` | 字段、指标和人工审核校验已迁移。可编辑报告和线上审批入口待实现 |
| 测试申请、计划/评审、准入准出、日报与正式总结导出 | `design/2026-08-ai-workflow-upgrade/requirements.md` | — | 主平台目前尚无完整企业流程实体和正式导出物，旧 React 页面也未提供可复用的后端实现 |
| React/Node 服务、演示数据、部署脚本 | 根目录 Node/React 工作台 | — | 不迁移第二套服务或产品前端 |

## 后续迁移

- `record-studio` 的套件并发、步骤状态与耗时、CLI 录制及工作台 JSON 导入已进入主平台。后续仍需一键发起录制、节点属性继承、动态 API 变量与逐步截图。
- `laya-pilot` 的单层 iframe、可访问行上下文、开放式 Shadow DOM、原生下拉及复选框已接入结构化回放。旧格式工作簿和其他复杂控件仍待迁移。
- `lemon-TestPilot` 的七类企业测试产物已支持草稿、认证签核、版本统计和 HTML 导出。静态 AI 入口目录到 Deep Agents 的各类产物生成仍需接入。
- Laya 迁入实现的 MIT 许可证和归因保留在工作簿模块。旧运行壳、原始参考资料与运行数据保留在本地。

## 离线用例转换

在仓库根目录运行以下命令。

```powershell
python scripts/case_cli.py convert-recorded events.json scenario.json
python scripts/case_cli.py export-xlsx scenario.json case.xlsx
python scripts/case_cli.py import-xlsx case.xlsx restored.json
```

录制转换后需在用例工作台补充断言、绑定测试数据并完成评审。可见步骤有改动时，以场景 JSON 重新导出工作簿，保证回放数据与审核内容一致。
