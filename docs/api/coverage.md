# 版本白盒覆盖率

本轮交付覆盖率证据导入与版本汇总。平台解析 CI 或测试工具生成的文本，持久化租户、项目、版本、格式与数值计数。输入仅在请求内存中处理，原始报告、文件名、路径、源码与命令均不落盘或回显。完整白盒执行范围仍待确定。

## 统计口径

- Go coverprofile 支持 `set`、`count`、`atomic`。按代码块的语句数累加，命中次数大于零的块计入覆盖。相同文件及起止位置的重复块只计一次，任一命中即视为该块命中。重复块的语句数不一致时拒绝导入。
- Cobertura 从 `packages/package/classes/class/lines/line` 读文件行，按 `filename` 和行号去重，`hits > 0` 为命中。忽略方法内重复记录和 XML 的比率汇总。对 `branch="true"` 的行从 `condition-coverage` 的括号内读取覆盖数与总数。重复行的分支计数必须一致，否则拒绝导入。没有分支时返回 `0/0`。
- 两种格式各统计各自的单位。服务端保留同版本同格式的最新有效汇总，重新导入时整体替换。读取或导出报告时可按 `covered / total` 计算比例，分母为零时显示 `—`。

## 接口与错误

`GET /api/versions/{id}/coverage` 返回汇总数组。`POST /api/versions/{id}/coverage` 使用 JSON `{ "format": "go-coverprofile", "content": "mode: set\nexample/demo.go:1.1,2.1 1 1\n" }`。两者复用版本接口的租户鉴权，项目由版本确定。访问不存在或不属于当前租户的版本返回 `404`。

无效 JSON 或缺少字段返回 `400`。报告文本超过 1 MiB 或 JSON 请求体超过 2 MiB 返回 `413`。格式不匹配、没有覆盖率记录、计数非法或 XML 不完整返回 `422`。无效导入不影响成功保存的汇总。详细字段见 [OpenAPI](coverage.openapi.yaml)。

## 集成

1. 在 `store.Open` 现有迁移流程调用 `MigrateCoverage(db)` 并返回迁移错误。
2. 在 `Server.Routes` 调用 `s.registerCoverage(mux)`，注册版本覆盖率的 `GET` 和 `POST` 路由。
3. 在 `VersionsView.vue` 导入 `CoveragePanel`，于版本表格下方挂载 `<CoveragePanel :versions="rows.filter(row => row.project_id === projectId)" />`。
4. 若工作台报告需要展示该结果，在现有报告统计事务内调用 `store.ListVersionCoverage(ctx, tx, tenantID, versionID)`，将返回值作为独立的白盒覆盖率字段保存到报告快照。
