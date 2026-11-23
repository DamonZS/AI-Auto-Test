# AI 合成测试数据

`POST /api/test-data/generate` 使用现有会话和 `X-Tenant-Id`，需要工作区写权限。接口用于常规功能、边界和异常场景的合成数据，生成后直接保存。

```json
{"project_id":"project-id","environment_id":"environment-id","fields":["customer_name","password","amount"],"prompt":"至少一行包含负金额，至少一行包含空密码","count":3}
```

- `project_id` 为必填项。可选的 `environment_id` 必须属于当前租户和所选项目。
- `count` 为 1–30 的整数。`fields` 可选，选中时须传下表中的唯一键。`prompt` 可选，与 `fields` 至少提供一项。请求体上限为 32 KiB。
- `prompt` 可描述密码字段和普通测试样例。请用字段名和测试条件描述需求。显式凭据值或 URL 会被拒绝。上游消息只含提示、行数与所选字段契约。
- 成功返回 `201` 和 `{"id":"dataset-id","name":"测试数据集","record_count":3}`。列表 `GET /api/test-data/datasets` 返回 `generation_source`，旧数据集和手工创建的数据集为 `rule`，新接口生成的数据集为 `ai`。GUI 仅对 `rule` 展示规则重新生成。`POST /api/test-data/datasets/{id}/generate` 对 `ai` 返回 `409`。

| 分类 | 字段键与显示名 |
| --- | --- |
| 客户信息 | `customer_name` 客户姓名、`customer_id` 客户编号、`document_type` 证件类型、`document_number` 证件号码、`company_name` 公司名称、`birth_date` 出生日期、`member_level` 会员等级 |
| 登录认证 | `username` 用户名、`password` 密码、`email` 邮箱、`phone` 手机号、`account_status` 账号状态 |
| 联系地址 | `contact_name` 联系人、`phone` 手机号、`province` 省份、`city` 城市、`address` 详细地址、`postal_code` 邮政编码 |
| 订单交易 | `order_number` 订单编号、`amount` 订单金额、`quantity` 商品数量、`order_date` 下单日期、`order_status` 订单状态 |

选择字段后，服务端按选择顺序定义字段，模型返回数据集名称及相同键的行。仅使用提示时，模型也返回 `fields` 定义，类型必须为 `name`、`email`、`phone`、`number`、`date`、`uuid`、`text` 或 `password`。每行必须恰好包含全部字段，允许空字符串与负数。数字可来自 JSON 数字或数字字符串，保存为现有记录接口使用的字符串。非空日期用 `YYYY-MM-DD`。

普通记录接口以 `••••••` 显示敏感字段。服务端替换非空密码、电话和证件号码。按数据集从第一行起依次使用 `SyntheticQA-0001!`、北美保留测试号 `+1-202-555-0100` 起的号码和 `TEST-DOC-0001`。执行端的原始记录接口可核对这些固定值。空字符串仍保存在原始记录中。

| 状态 | 含义 |
| --- | --- |
| `400` | 无效 JSON、行数或选字段参数 |
| `401` / `403` | 未登录或工作区权限不足 |
| `404` | 项目或所选环境不属于当前作用域 |
| `422` | 含有显式凭据或 URL，或缺少可用的租户模型配置 |
| `502` | 模型请求失败或输出校验失败 |
| `500` | 读取配置或保存失败 |

生成失败时不保存数据集和记录。现有 SQLite 库启动时添加 `generation_source` 列，已有行按默认值读取为 `rule`。部署前备份数据库及配套密钥文件。若回滚至旧服务版本，使用部署前的备份，避免旧规则生成入口覆盖后续创建的 AI 数据。
