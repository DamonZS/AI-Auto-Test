## 项目架构

```
auto_ai/
├── auto_ai/                 # 核心包
│   ├── __init__.py
│   ├── config.py               # 配置管理（环境变量 → LLM / Neo4j）
│   ├── snapshot.py             # 页面感知层：快照、指纹、缓存、区域识别
│   ├── browser.py              # 浏览器层：Playwright 生命周期 + 原子操作 + API 追踪
│   ├── agent.py                # Agent 层：LangGraph 工作流 + 自愈 + 断言 + 崩溃
│   └── knowledge_graph.py      # Neo4j 知识图谱（页面拓扑、元素、API 关联）
├── scripts/                 # CLI 入口
│   ├── main.py                 # 单场景调试
│   └── kg_cli.py               # 知识图谱调试工具
├── tests/                   # pytest 测试用例
│   ├── conftest.py
│   └── test_scenarios.py
├── docs/
│   └── ARCHITECTURE.md
├── cache/
│   └── page_templates.json
├── screenshots/
├── pyproject.toml
├── requirements.txt
└── .env / .env.example
```

---

## 分层架构

```
┌──────────────────────────────────────────────────┐
│                    main.py                        │
│              CLI 参数解析 + 启动编排               │
└────────────────────┬─────────────────────────────┘
                     │
┌────────────────────▼─────────────────────────────┐
│                   agent.py                        │
│            LangGraph 工作流（AI 大脑）              │
│                                                    │
│   Planner ──→ Executor ──→ Router ──→ Reporter    │
│   (拆解步骤)   (执行+自愈)   (路由+crash) (汇总报告) │
│                                                    │
│   调用 LLM: deepseek-chat（规划 + 断言 + 重定位）   │
└───────┬────────────────────────────┬──────────────┘
        │                            │
┌───────▼──────────┐    ┌────────────▼──────────────┐
│   snapshot.py    │    │       browser.py           │
│   页面感知层      │    │       浏览器操作层          │
│                  │    │                            │
│ · accessibility  │    │ · 生命周期管理              │
│   tree 提取      │    │ · click_smart / fill_smart │
│ · 精简过滤       │    │ · _locate_element（降级链） │
│ · 定位器补刀     │    │ · APITracker（请求追踪）    │
│ · 指纹计算       │    │ · 截图 / wait / navigate   │
│ · 缓存读写       │    │                            │
│ · 区域（Section）│    │                            │
└───────┬──────────┘    └────────────┬───────────────┘
        │                            │
        └──────────┬─────────────────┘
                   │
        ┌──────────▼──────────┐
        │     Playwright       │
        │   Chromium 浏览器     │
        └──────────────────────┘
```

---

## 各模块职责

### 1. `config.py` — 配置管理

从环境变量 `.env` 读取所有配置，集中管理：

| 变量 | 作用 | 默认值 |
|------|------|--------|
| `OPENAI_API_KEY` | DeepSeek/OpenAI API Key | - |
| `OPENAI_BASE_URL` | API 地址 | `https://api.deepseek.com` |
| `PLANNER_MODEL` | 规划步骤用的模型 | `deepseek-chat` |
| `EXECUTOR_MODEL` | 断言分析 / 自愈重定位用的模型 | `deepseek-chat` |
| `NEO4J_ENABLED` | 是否启用知识图谱 | `true` |
| `NEO4J_URI` | Neo4j 连接地址 | `bolt://localhost:7687` |
| `NEO4J_PROJECT` | 项目标签（数据隔离） | `auto_ai` |

支持任意 OpenAI 兼容接口（DeepSeek / OpenAI / 本地模型）。

---

### 2. `snapshot.py` — 页面感知层

负责将浏览器页面转化为 LLM 可理解的低成本文本。

**核心函数：**

| 函数 | 功能 |
|------|------|
| `normalize_url(url)` | URL 去参数，保留路径骨架 → `/login` |
| `make_fingerprint(snapshot)` | 提取页面"骨架"做 MD5 hash → `a3f2b8c1` |
| `slim_snapshot(snapshot)` | 精简 accessibility tree，去噪音、只留可交互元素 |
| `enrich_with_locators(tree, page)` | 为每个交互元素补 `get_by_role` 定位器 |
| `get_snapshot(page)` | 获取当前页快照，优先走指纹缓存 |
| `take_screenshot(page, name)` | 保存截图到 `screenshots/` |

**Token 优化链路：**

```
页面加载
  → accessibility.snapshot()        （不是 page.content()，体积小 10-50 倍）
  → enrich_with_locators()           （补定位器，0 token）
  → make_fingerprint()               （算指纹）
  → 查缓存（命中 → 0 token；未命中 → 继续）
  → slim_snapshot()                  （去噪音，只留 [role] name）
  → 返回精简文本给 LLM                （通常 500-2000 字符）
```

**缓存机制：**

- 内存缓存：`_cache` dict，O(1) 查找
- 磁盘持久化：`cache/page_templates.json`，下次启动复用
- Key = `normalize_url(url)`，同路径不同参数共享一个缓存
- 校验 = 指纹匹配，页面结构变了自动更新
- `force_refresh=True`：跳过缓存，强制重新提取 DOM 并更新缓存

**区域识别（Section）：**

JS 注入时自动向上查找祖先容器（nav/form/table/header/footer/aside/main/section），元素按区域分组，未匹配的归入"其他"。

---

### 3. `browser.py` — 浏览器操作层

封装 Playwright 生命周期和原子操作，对外只暴露方法名 + role + name。

**BrowserSession 类：**

```python
session = BrowserSession(headless=True)
await session.start()
# ... 执行测试 ...
await session.stop()
```

**原子操作：**

| 方法 | 说明 |
|------|------|
| `navigate(url)` | 导航并返回页面快照（networkidle + 轮询元素计数） |
| `click_smart(role, name)` | 智能点击：多策略定位 + 自愈降级 |
| `fill_smart(role, name, value)` | 智能填入：同上 |
| `get_page_state(force_refresh)` | 获取当前页快照，force_refresh 强制刷新 |
| `wait_for(text)` | 等待文本出现 |
| `screenshot(name)` | 截图到 `screenshots/` |
| `get_current_url()` | 获取当前页面完整 URL |
| `check_element_attribute(name, attr)` | 检查元素属性（disabled/enabled/等） |
| `count_table_rows(name)` | 统计表格行数 |

#### 元素定位降级链（`_locate_element`）

纯代码，0 token，内置 5 级降级：

```
① get_by_role(role, name)       → 精确 role 匹配
② get_by_text(name, exact=True) → 精确文本匹配
③ get_by_text(name)             → 模糊文本匹配
④ 滚动页面 + 重试 ①②③（最多 3 次）→ 处理懒加载/折叠区域
⑤ 放弃 → 返回 None（触发 LLM 兜底）
```

每次定位都会记录 `last_locate_detail = {"tried": [...], "used": "strategy"}`，失败时输出到报告。

#### API 追踪（APITracker）

每次 click/fill 操作自动捕获触发的 XHR/Fetch 请求：

| 字段 | 说明 |
|------|------|
| `url` | 请求地址 |
| `method` | GET/POST |
| `post_data` | 请求体 |
| `status` | 状态码（含 4xx/5xx 错误） |
| `body` | JSON 响应体（成功和错误都抓，限 100KB） |

`get_apis()` 在失败步骤上自动挂载，报告中显示 `status >= 400` 的错误 API。

---

### 4. `agent.py` — LangGraph Agent 层

#### 工作流图

```
                    ┌──────────┐
                    │  START   │
                    └────┬─────┘
                         │
                    ┌────▼─────┐
                    │ Planner  │   LLM 调用 1 次
                    │ (规划)   │   场景 → 步骤 JSON
                    └────┬─────┘
                         │
                    ┌────▼─────┐
              ┌─────│ Executor │◄──────────┐
              │     │ (执行+自愈)│           │
              │     └────┬─────┘            │
              │          │                  │
              │     ┌────▼─────┐            │
              │     │  Router  │            │
              │     │ (路由)   │            │
              │     └────┬─────┘            │
              │          │                  │
              │  ┌───────┼───────┐          │
              │  │       │       │          │
              │  ▼       │       ▼          │
              │ "execute"│   "report"       │
              │ 还有步骤  │ 完成 / crash     │
              │          │                  │
              └──────────┘      ┌────▼─────┐
                                │ Reporter │  汇总 + 上下文 + 截图
                                │ (报告)   │
                                └────┬─────┘
                                     │
                                ┌────▼─────┐
                                │   END    │
                                └──────────┘
```

#### 节点说明

**Planner（规划器）**

- 输入：`test_scenario`（自然语言）+ `target_url`
- 输出：结构化步骤数组 `[{action, target, role, value, description}]`
- LLM 调用：1 次
- 输出校验：3 层（JSON 安全解析 → 步骤字段校验 → 自动补 navigate 首步）

**Executor（执行器）**

- 循环执行 `state.steps[state.current_step]`
- 根据 `action` 字段分派：

| action | 操作 | token |
|--------|------|:-----:|
| `navigate` | 导航 + 快照 | 0 |
| `click` | 智能点击 + 自愈 | 0（失败时 LLM 1次） |
| `fill` | 智能填入 + 自愈 | 0（失败时 LLM 1次） |
| `wait` | 等待文本出现 | 0 |
| `assert` | 断言（见下文断言链） | 0 / LLM |

- 每步失败时自动挂载完整上下文（见"失败上下文"章节）

**Router（路由器）**

- 还有未执行步骤 → 回到 Executor
- 全部完成 → 进入 Reporter
- 检测到 `state["error"] == "crash"` → 立即终止 → Reporter

**Reporter（报告器）**

- 汇总 `state.history`，统计通过/失败/崩溃
- 失败步骤展示：页面 URL、定位策略链、页面快照、截图、API 错误
- 自愈成功步骤标记 ♻
- 崩溃时显示中断提示
- Token 消耗统计

---

### 5. 自愈机制（Self-Healing）

双层自愈，只在操作失败时触发：

```
click / fill 失败
  │
  ├── 第一层（代码）：_locate_element 5 级降级链（0 token）
  │     ├─ 命中 → 执行成功
  │     └─ 全部失败 → 进入第二层
  │
  └── 第二层（LLM）：_llm_relocate 兜底（1 次 LLM 调用）
        ├─ force_refresh 拉最新 DOM + 更新缓存
        ├─ LLM 看最新快照 → 建议替代元素定位
        ├─ 用新定位重试一次
        │    ├─ 成功 → status=ok, _self_healed=True
        │    └─ 失败 → status=fail（这就是 bug）
        └─ 不循环，只试一次
```

关键设计：正常路径优先走缓存，只有失败才 force_refresh 拉 DOM 给 LLM 分析。

---

### 6. 断言链（Assert Chain）

6 种断言类型，代码优先（0 token）→ LLM 兜底：

```
① URL 匹配      → 代码，0 token  |  assert "/dashboard"
② API 状态码    → 代码，0 token  |  assert "API /api/login 返回 200"
③ 表格行数      → 代码，0 token  |  assert "table 货源列表 行数 >= 5"
④ 元素属性      → 代码，0 token  |  assert "提交" disabled
⑤ 关键词匹配    → 代码，0 token  |  assert "欢迎回来"（快照中查找）
      ↓ 匹配不到 ↓
⑥ LLM 断言      → LLM 兜底       |  复杂语义
```

检测逻辑：`_detect_assert_type(target)` 由正则判断类型 → 分流到对应处理函数。

---

### 7. 失败上下文与崩溃检测

#### 失败步骤自动挂载的上下文

| 字段 | 说明 |
|------|------|
| `page_url` | 失败时的页面 URL |
| `page_snapshot` | 失败时的页面快照（截断 500 字符） |
| `screenshot` | 失败时的截图（`step_N_fail.png`） |
| `api_trace` | 触发的 API 调用（含 4xx/5xx 错误响应） |
| `locate_detail` | 定位策略尝试链（click/fill 专用） |

#### 崩溃检测

| 状态 | 含义 | 行为 |
|------|------|------|
| `fail` | 操作/断言不通过 | 继续执行后续步骤 |
| `error` | 代码异常（非崩溃） | 继续执行后续步骤 |
| `crash` | 浏览器/页面不可恢复 | **设置 `state["error"]`** → Router 立即终止，防止后续步骤在坏页面上连续报错 |

崩溃判断依据（`except` 块中异常类型名匹配）：
- `TargetClosed` / `TargetCrashed` / `ConnectionClosed` / `BrowserClosed`
- 导航时的 `TimeoutError`

---

### 8. `scripts/main.py` — CLI 入口

CLI 参数：

```
--url           目标 URL
--scenario      测试场景（自然语言描述）
--headless      无头模式（默认开启）
--no-headless   显示浏览器窗口（调试用）
--cache-dir     缓存目录（默认 cache/）
```

使用示例：

```bash
python scripts/main.py --url "https://example.com/login" --scenario "用 admin/123456 登录，验证跳转到首页"
```

---

### 9. `tests/` — pytest 测试运行器

CLI 用于单场景调试，pytest 用于批量回归测试。`run_test()` 返回 `(report, passed)`，pytest 根据 `passed` 判定 PASSED / FAILED，失败时完整报告会出现在 pytest 输出中。

**conftest.py：** 提供 `session` fixture，管理浏览器生命周期。

**test_scenarios.py：** 每个 `async def test_xxx(session)` 是一个独立的测试用例。

运行方式：

```bash
pytest tests/ -v                          # 全部用例
pytest tests/ -v -s                       # 显示 print 输出（含报告）
pytest tests/ -v -k test_login            # 按名称筛选
pytest tests/ --html=report.html          # 生成 HTML 报告
```

批量参数化（可选）：

```python
import pytest

@pytest.mark.parametrize("url,scenario", [
    ("https://example.com/login", "用 admin/123456 登录，验证跳转首页"),
    ("https://example.com/login", "用错误密码登录，验证提示'密码错误'"),
])
async def test_login(session, url, scenario):
    report, passed = await run_test(session, url, scenario)
    assert passed, f"\n{report}"
```

---

### 10. `knowledge_graph.py` + `kg_cli.py` — 知识图谱（可选）

Neo4j 数据模型：

```
(Page) ←[PART_OF]— (Section) ←[IN_SECTION]— (Element)
  │                                              │
  └──[NAVIGATES_TO]→ (Page)                      └──[TRIGGERS_API]→ (API)
```

所有节点带 `project: "auto_ai"` 属性隔离，支持 `NEO4J_ENABLED=false` 关闭。

`kg_cli.py` 提供调试工具：`python scripts/kg_cli.py pages|elements|nav|apis|stats|clear`

---

## Token 消耗分析

以 10 步测试用例（含 3 次 assert）为例：

| 环节 | LLM 调用次数 | 单次 Token | 合计 |
|------|:---:|:---:|:---:|
| Planner | 1 | ~800 | 800 |
| Assert（代码命中 2 次，LLM 兜底 1 次） | 1 | ~600 | 600 |
| click / fill / wait | 0 | 0 | 0 |
| 存储命中页面 | 0 | 0 | 0 |
| **总计** | **2** | | **~1,400 Token** |

> 注：大部分 assert 走代码（0 token），只有代码匹配不到才降级 LLM。自愈 LLM 只在定位失败时触发，正常流程不消耗。

DeepSeek V3 价格 ¥1/百万 token → 单次测试约 **¥0.0014**。

---

## 报告示例

```
==================================================
  自动化测试报告
==================================================
测试场景：用 admin/123456 登录，验证跳转到首页
目标 URL：https://example.com/login
总步骤数：4
结果：✗ 存在失败

  ✓ 步骤 1: [navigate] https://example.com/login
      导航到 https://example.com/login

  ✗ 步骤 2: [click] 提交
      失败：自愈定位全部失败，未找到 '提交' (role=button)
      页面: https://example.com/login
      定位策略: role(button, 提交) → text_exact(提交) → ... → scroll3_text_fuzzy → 全部失败
      截图: screenshots/step_2_fail.png
      快照: [heading] 登录 [textbox] 用户名 [textbox] 密码 [button] 确认...
      API错误: POST /api/login → 500 {'error': 'Internal Server Error'}

  ♻ 步骤 3: [fill] 用户名
      成功：通过 [text_fuzzy] 在 '用户名' 中填入了 'admin'
      ♻ 已通过自愈重试恢复

  ✓ 步骤 4: [assert] 欢迎回来
      API错误: GET /api/user → 401 {'message': 'Unauthorized'}

失败步骤数：1
```

---

## 设计决策

1. **不用 MCP，自定义 Playwright 封装**：MCP 的设计目标是"让 LLM 自由操作浏览器"，而本方案的核心思路是"尽量不让 LLM 操作浏览器"，两者方向相反。自定义封装可以精确控制 token 消耗。

2. **用 accessibility tree 而非 HTML**：体积缩小 10-50 倍，天然只包含可交互元素，与 ARIA role 定位完美配合。

3. **分层模型策略**：Planner 和 Assert/Relocate 可以分别配置不同模型，规划用强模型保证步骤质量，断言和重定位用便宜模型省钱。

4. **指纹缓存而非完整 URL 缓存**：同路径不同参数的页面共享缓存，页面结构变了自动更新，兼顾命中率和准确性。

5. **步骤即断言**：关键操作后自动跟 assert 步骤，代码优先匹配（0 token），匹配不到才降级 LLM。

6. **崩溃立即终止**：区分 crash（浏览器挂了）和 fail（操作不通过），crash 时设置 `state["error"]` 让 Router 立即跳 Repo，避免后续步骤在坏页面上产生假失败。

7. **自愈只试一次**：定位失败 → 刷新 DOM → LLM 重定位 → 重试一次 → 再失败就是 bug，不循环。
