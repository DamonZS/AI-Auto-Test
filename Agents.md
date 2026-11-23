# Autocomplete-Test 项目记忆

> 本项目所有会话中的重要记忆、需求、需求变更都记录在本文件。

## 项目定位
基于 AI 的自愈性 UI 自动化、接口自动化、接口性能企业级 Agent 测试平台，包含三大能力：
1. UI 自动化
2. 接口自动化
3. 接口性能测试

## 当前目录现状（2026-11-21 观察）
- 子目录：auto_ai、cache、docs、laya-pilot-main、lemon-TestPilot-main、record-studio、reports、screenshots、scripts、tests
- 根目录文件：.env、.env.example、pyproject.toml、requirements.txt、run_tests.ps1
- 各子目录职责见下方「现状梳理」。

## 需求记录
- [2026-11-21] 初始需求：做一个软件测试平台，覆盖 UI 自动化、接口自动化、接口性能测试三类测试。
- [2026-11-21] 约定：所有项目需求、需求更改都写入本文件（Agents.md）。

## 需求变更记录
- [2026-11-23] 用例可从已批准的需求功能点评审生成，支持有序步骤与预期结果的手工功能、UI、接口、有上限的性能以及固定清单的防御性安全审查。审核退回需要反馈，AI 修订形成下一内容版本，重新提交后由会话用户定稿。已定稿的手工功能和安全审查可直接登记人工通过或失败结果。原因是保留需求、反馈和验证证据的追溯关系。
- [2026-11-23] 测试数据默认使用 AI 生成。默认勾选「客户信息」「登录认证」，预设可组合，字段可单独勾选，有选中字段即可省略需求描述。支持按项目和可选环境生成 1–30 条，「生成并保存」后自动打开记录。AI 数据集标明来源并隐藏手动「重新生成」，手动按规则生成仍可切换使用。原因：减少生成并入库测试数据所需的配置步骤。
- [2026-11-21] 定位补充：平台是基于 AI 的自愈性 UI 自动化、接口自动化、接口性能的企业级 Agent 测试平台，不只是普通测试平台。原因：用户补充说明。
- [2026-11-21] 待办：先梳理现有代码与文档，再确定最优架构。待选型项：framework 还是 runtime、harness 还是 ReAct、plan-and-execute、multi-agent、reflective、tool、memory、RAG、autonomous loop、LangChain、LangGraph 等。结论确定后写入本文件。

## 会话重要记忆
- 与用户沟通使用中文。
- 需求变更发生时，在上方 需求变更记录 追加一条，注明日期、变更内容、原因。

## 现状梳理（2026-11-21）
- auto_ai/ + tests/ + scripts/：自研 UI 自动化核心。LangGraph 的 Planner -> Executor -> Router -> Reporter；5 级元素定位降级链（0 token）+ LLM 重定位自愈（只试 1 次）；断言链（代码优先、LLM 兜底）；页面快照指纹缓存；可选 Neo4j 知识图谱；Excel 用例 + pytest 批量。agent.py 953 行，仅覆盖 UI。
- record-studio/：Python FastAPI + Vue 3 + Playwright 录制回放工作台，SQLite 持久化，已有用例、套件、并发、调度器（scheduler.py）、节点（nodes.py）。平台外壳雏形已具备。backend 下散落大量下划线开头的临时脚本、txt 和备份文件，需清理。
- lemon-TestPilot-main/：Node + React 个人测试工作台（用例树、Bug/日志/SQL、版本化知识库、AI 助手编排、人工复核边界）。借鉴产品设计，不作代码底座（Node 技术栈，面向个人）。
- laya-pilot-main/：Node + Python，从真实页面生成可回放 Excel 用例；模型只做歧义决策，结果必须由页面断言确认。借鉴「生成与回放分离、模型选择不算通过」的原则。
- 缺口：接口自动化、接口性能测试目前没有任何代码；自愈没有「脚本漂移 vs 真实缺陷」的判别，也没有回写与审批；没有多用户、权限、任务队列、审计。
- 风险：auto_ai/config.py 里 NEO4J_PASSWORD 的默认值写死了密码，需改为无默认值、只读环境变量；.env 不入库。

## 架构草案（历史记录，已被下方「架构定稿」取代）
- 结论：LangGraph 做编排 runtime + 自研确定性执行 harness；ReAct 只用在有预算上限的诊断/探索节点里；不做无限制自主循环。
- 分层：1 控制面（FastAPI + Vue 3 + 任务队列 + DB，沿用 record-studio 外壳）；2 Agent 层（Supervisor 图按任务路由到 UI / 接口 / 性能三个子图，各子图 plan-and-execute，带有限次 reflect 节点）；3 工具层（Playwright、HTTP 客户端、Locust 或 k6、DB 查询，统一工具接口）；4 记忆与知识；5 治理（审计、token 预算、轨迹回放、凭据管理、人工审批）。
- 自愈分级：代码降级链 -> 历史定位器 / DOM 差异 -> LLM 重定位 -> 用断言验证 -> 判别「脚本漂移」还是「产品缺陷」-> 修复只生成待审补丁，批准后才写回定位器库，不静默改用例。
- 记忆：短期用 LangGraph state + checkpoint；长期为定位器库、自愈历史、失败模式（关系库）；RAG 只用于需求文档、OpenAPI/Swagger、历史缺陷；Neo4j 保持可选，放在接口后面。
- 接口自动化：由 OpenAPI/抓包生成用例，断言代码优先，LLM 只做用例生成与失败归因。
- 性能测试：LLM 不直接产生负载，只生成场景脚本/配置，由 Locust 或 k6 执行，LLM 分析结果并给出瓶颈假设。
- 不采用：纯 ReAct 执行（token 与不确定性高，无法做 CI 门禁）；LangChain 高层 Agent 封装（只保留 langchain-openai 模型适配）；无限自主循环。
- 待用户确认：(1) 后端统一 Python，以 record-studio + auto_ai 为底座，lemon-TestPilot 与 laya-pilot 仅作参考；(2) 部署形态（单机 / 企业多租户）；(3) 向量库与任务队列选型。

## 选型决策（2026-11-21）
- [2026-11-21] 决策：Agent 层 harness 选定 Deep Agents（基于 LangGraph，Python）。原因：与 auto_ai / record-studio 同为 Python；自带规划、子 Agent、上下文文件系统；可直接用 LangGraph 的 checkpoint 与人工中断；模型不绑定，继续使用 DeepSeek 等 OpenAI 兼容接口。
- 备选：Claude Agent SDK。仅当 Deep Agents 的失败归因质量不足时，单独接入「失败归因」节点，不作全局 harness。
- 未纳入比较：用户列出的 herness agent、deepseek harness，名称无法确认。用户提供链接后再用 gitingest 评估，不影响当前决策。
- 不采用：OpenCode、Pi、OpenClaw（TS 栈，面向编码终端或个人助理，与受控测试执行不匹配）。
- 硬约束：(1) Deep Agents 默认偏自主循环，必须配置步数、token、时间上限；(2) 自愈修复只生成待审补丁，批准后才写回定位器库；(3) 5 级定位降级链、断言链、Playwright、HTTP、Locust 或 k6 封装成确定性工具，不交给模型决定；(4) 所有 Agent 轨迹可回放、可审计。
- 影响：auto_ai/agent.py（953 行）需要迁移，不是零改动。

## 待办
- [ ] Spike（1 到 2 天）：用 Deep Agents 接管 auto_ai 现有登录场景，复用现有 Playwright 工具，对比 token 消耗、自愈成功率、可复现性。明显差于原 LangGraph 实现则退回「自研 LangGraph + Supervisor」，并在本文件记录原因。
- [x] auto_ai/config.py：去掉 NEO4J_PASSWORD 默认值，只读环境变量（2026-11-21 已完成，默认值改为空字符串）。
- [ ] 清理 record-studio/backend 下划线开头的临时脚本、txt、备份文件（先列清单给用户确认再删）。
- [ ] 起接口自动化子图骨架。

## 仍待用户确认
- 后端统一 Python，以 record-studio + auto_ai 为底座：已确认（见「用户确认」）。
- 部署形态：已确认企业多租户（见「用户确认」）。
- 向量库与任务队列：已确认先用 SQLite + Chroma，队列先用进程内（见「用户确认」）。

## 用户确认（2026-11-21）
- 部署形态：企业多租户。影响：数据按租户隔离（tenant_id）、权限（RBAC）、凭据按租户管理、token 预算与任务配额按租户、审计日志按租户。
- 存储：先用 SQLite（关系数据）+ Chroma（向量 / RAG）；任务队列先用进程内，后续再换 Celery 或 Redis。
- 风险：SQLite 是单文件库，并发写入弱，多租户同时跑任务时会成为瓶颈，Chroma 本地模式同理。先用可以，但所有表从第一天起带 tenant_id，数据访问统一走一层，后续迁移 PostgreSQL（+ pgvector）时不改业务代码。迁移触发条件待与用户商定。
- 后端统一 Python（以 record-studio + auto_ai 为底座）：用户选择 A 方案，已确认定稿。record-studio（FastAPI + Vue 3）作平台外壳，补多租户、权限、审计；auto_ai（LangGraph + Playwright）作 UI 自动化引擎，Agent 层迁到 Deep Agents；lemon-TestPilot、laya-pilot 为 Node 项目，只借鉴设计，不合并代码。不推倒重写，不引入第二套后端技术栈。

## Spike 任务拆分（待用户说「开始」后由便宜模型的子智能体并行执行）
1. 在独立目录安装 deepagents，确认能用 DeepSeek 的 OpenAI 兼容接口跑通最小示例，记录版本与依赖冲突。
2. 把 auto_ai/browser.py 的 click_smart、fill_smart、navigate、get_page_state 封装成 Deep Agents 可调用的工具，不改原逻辑。
3. 用 Deep Agents 跑现有登录场景，配置步数、token、时间上限，记录 token 消耗、步数、成败。
4. 用原 auto_ai LangGraph 实现跑同一场景，同样记录。
5. 对比结果，写入本文件：token、自愈成功率、可复现性（各跑 5 次）。结论为采用或退回。

## 需求变更：UI 自动化分两档（2026-11-21）
- [2026-11-21] 用户要求：UI 自动化初步使用 ego (lite) 浏览器；高性能档使用 laya，或接入 jev 决策判断型模型做自动化。
- 已知：laya 指 laya-pilot-main 中使用的本地决策模型（Laya，用于消解有歧义的控件）。jev 在 laya-pilot 的 README 里只列在 Roadmap，当前未实现，只实现了 Playwright 浏览器驱动和 Laya 决策协议。
- 未确认：ego (lite) 浏览器具体指哪个产品或项目，工作区内未确认，待用户说明或提供链接，不得凭名字假设其接口与能力。
- 设计含义（待确认）：UI 层需要把浏览器驱动（lite 档：ego (lite)，现有：Playwright）和决策模型（高性能档：laya / jev）做成可切换的 provider，由配置按租户或任务选择档位；确定性执行路径与断言规则不随档位改变，模型选择不算通过，结果以页面断言为准。
- 与已定决策的关系：Deep Agents 仍负责编排，laya / jev 只负责元素与动作的决策判断，作为工具层的一个 provider，不替代 Deep Agents。
- 对 Spike 的影响：Spike 先按现有 Playwright 工具进行，不等 ego (lite) 的信息；provider 抽象待用户确认 ego (lite) 后再设计。
- 待用户确认：(1) ego (lite) 是什么，如何接入；(2) 两档如何切换（按租户套餐、按任务，还是自动升档）；(3) jev 是自建模型服务还是第三方 API，接口文档在哪。

## 需求变更：浏览器驱动必须同时适配 Playwright 与 ego (lite)（2026-11-21）
- [2026-11-21] 用户要求：UI 自动化除 ego (lite) 外，必须同时适配 Playwright。原因：部分企业无法下载 ego，且要求使用特定浏览器。
- 修订：此前「lite 档使用 ego (lite)」的表述不再成立，ego 不再是唯一的浏览器驱动。Playwright 是必选基线，ego (lite) 是可选增强档。
- 设计含义：浏览器驱动做成 provider 抽象。Playwright provider 必须能独立完成全部 UI 功能（服务端、CI、Linux、可指定企业要求的浏览器通道），不依赖 ego。确定性执行路径、断言规则、自愈补丁审批流程与 provider 无关。ego 不可用时，按配置降级到 Playwright。
- ego (lite) 已知事实（官网与 GitHub，2026-11-21 读取）：基于定制 Chromium 的桌面应用；通过 ego-browser Skill 与 Agent 配合，使用 helper 函数、浏览器内 js()、原始 CDP；未见 Playwright 兼容接口与 Python SDK，需读文档核实；Windows 已支持，Linux 在路线图中；MIT 许可；官方定位不是测试运行器。
- 对 Spike 的影响：Spike 继续使用 Playwright，不受影响。
- 待用户确认：(1) 两档按什么规则切换（租户套餐、任务指定、还是失败后升档）；(2) ego 是带界面的桌面应用，在企业多租户服务端与 CI 上是否可部署，需要确认后再决定它在平台里的位置。

## 需求变更：增强档范围（2026-11-21）
- [2026-11-21] 用户补充：laya 和 jev 也是增强档，不是基线。原因：基线必须在无法下载 ego、无法部署本地模型、只能用企业指定浏览器的环境里独立可用。
- 定稿的分层：基线档 = Playwright 浏览器驱动 + 确定性执行路径（5 级定位降级链、断言链）+ Deep Agents 编排（LLM 走 OpenAI 兼容接口，如 DeepSeek）。增强档 = 可选、可配置、可关闭的 provider。
- 增强档分两个维度，互相独立，可单独或组合启用：(1) 浏览器驱动增强：ego (lite)；(2) 决策判断增强：laya（本地或 HTTP API，laya-pilot 已实现）、jev（laya-pilot 路线图 P0，当前未实现，接口待确认）。
- 硬约束：任何增强档不可用、超时或报错时，必须按配置明确降级到基线，并在报告里记录降级原因；不允许静默回退，也不允许因为增强档缺失而无法执行测试。
- 硬约束：结果以页面断言为准，决策模型的选择不算通过；决策概率缺失或无效时不使用猜测分数；增强档的凭据不进日志、按租户管理。
- 设计含义：provider 抽象分两个接口，浏览器驱动接口（Playwright 必选实现，ego 可选实现）和决策接口（沿用 choose(state, criteria, meta) 候选加概率契约，laya / jev 为可选实现，基线可以不挂决策 provider）。
- 对 Spike 的影响：无，Spike 只用基线档。
- 待用户确认：(1) 增强档的启用粒度（租户套餐、项目、任务、还是失败后自动升档）；(2) jev 是自建模型服务还是第三方 API，接口文档在哪；(3) ego 是桌面应用，企业多租户服务端与 CI 能否部署，决定它放在服务端还是只放在测试人员本机。

## ego (lite) 接入核实（2026-11-21，来源：官方文档 ego-browser、Snapshot、DeepSeek Harness、自定义 Harness 四页与 GitHub README）
- 接入方式：命令行 ego-browser nodejs，脚本写在 heredoc 里，由 Node.js 进程执行，helper 函数（openOrReuseTab、snapshotText、click、fillInput、captureScreenshot 等）直接可用；js(字符串) 在浏览器页面上下文执行，本质是 Runtime.evaluate；cdp(方法, 参数) 可发原始 CDP 命令；输出只能用 cliLog。文档里没有 Python SDK，也没有 Playwright 兼容接口。
- Snapshot：基于无障碍树，约几百 token；@N 引用只对当前这一次快照有效，页面变化后失效；要稳定引用只能用快照里的 loc=... 或 CSS selector。因此用例回放不得保存 @N。
- 运行位置：桌面应用，必须在安装它的这台电脑上、沙箱外执行；任务执行期间会打开可见窗口；macOS 与 Windows 已支持，Linux 在路线图中；路线图还有「浏览器 ACP，可接入任何 Harness Agent」「可重复使用的技能」，均为进行中，未发布。
- 结论（待用户确认）：ego 只放在测试人员本机或带桌面的执行机上作增强档，不进入多租户服务端的无界面执行池，直到 Linux 版发布并实测无头运行。Python 侧用子进程调用 ego-browser nodejs 实现浏览器驱动 provider，需要单独验证稳定性、超时与进程回收。ego 的强项是探索与自愈时的页面理解，确定性回放仍以 Playwright 与稳定选择器为准。
- 官方文档的 Harness 页提到 DeepSeek Harness（npm 包 @deepseek-ai/dsh，本地 Web UI 与 headless 命令行）和 Hermes Agent。用户之前说的「deepseek harness」大概率就是前者；「herness agent」可能是 Hermes Agent，只是推测，待用户确认。两者都是与 Deep Agents 同层的 Agent harness，且 dsh 为 Node 栈。目前只读了 ego 文档对它们的描述，没有读它们自己的仓库，选型决策（Deep Agents）不变，用户给出链接后再评估。
- 待用户确认：(1) 上述 ego 定位；(2) herness agent 是否指 Hermes Agent。

## 用户确认（2026-11-21，续）
- [2026-11-21] ego (lite) 定位已确认：仅作增强档，部署在测试人员本机或带桌面的执行机，不进入多租户服务端的无界面执行池。Python 侧用子进程调用 ego-browser nodejs 实现浏览器驱动 provider，稳定性、超时、进程回收待单独验证。确定性回放以 Playwright 与稳定选择器为准，用例里不保存 @N 引用。Linux 版发布并实测无头运行后，再重新评估是否进入服务端执行池。
- [2026-11-21] 「herness agent」指 Hermes Agent；「deepseek harness」按 ego 文档推断为 DeepSeek Harness（@deepseek-ai/dsh），用户未再说明。用户明确：Agent 层 harness 定为 Deep Agents，不再评估这两个项目。选型为终局，除非后续出现明确的失败证据（见 Spike 的退回条件），不再重开讨论。
- 「仍待用户确认」里剩余项：(1) 增强档的启用粒度（租户套餐、项目、任务、失败后自动升档）；(2) jev 的接口形态与文档（自建服务还是第三方 API）。这两项不阻塞 Spike。
- 下一步：等用户说「开始」并给出测试站点（账号密码写本地 .env，不入库），再按 Spike 任务拆分用便宜模型的子智能体并行执行；NEO4J_PASSWORD 默认值清理与 record-studio/backend 临时文件清单可同时并行，清理只列清单，不直接删。

## 架构定稿（2026-11-21）
- 本节取代上方「架构草案」，分批写入：1 分层总览；2 端到端数据流；3 各模块处理；4 数据模型；5 自愈与审批；6 降级与预算；7 目录落位与迁移步骤。
- 前提：Python 后端；Deep Agents 为 Agent 层 harness；基线档 = Playwright + 确定性执行 + OpenAI 兼容 LLM；增强档（ego、laya、jev）可选、可降级；企业多租户；SQLite + Chroma；进程内队列。

### 1. 分层总览
```
控制面（record-studio 改造：FastAPI + Vue 3）
  登录与租户 | RBAC | 用例与套件 | 任务队列 | 调度器 | 报告 | 审批中心
        | 任务（tenant_id, type, 档位配置, 预算）
Agent 层（Deep Agents，基于 LangGraph）
  Supervisor --> UI 子 Agent / 接口子 Agent / 性能子 Agent
  规划(todo) | 子 Agent | 上下文文件系统 | checkpoint | 人工中断 | 预算上限
        | 工具调用
工具层（确定性，不交给模型决定）
  浏览器驱动接口：Playwright（基线）/ ego（增强）
  决策接口：laya / jev（增强，可不挂）
  HTTP 客户端 | Locust 或 k6 | 断言链 | 5 级定位降级链 | 快照与指纹缓存
        |
存储与知识
  SQLite（关系，全表带 tenant_id） | Chroma（需求文档、OpenAPI、历史缺陷）
  文件（截图、trace、报告） | Neo4j（可选）
治理（贯穿各层）：审计日志 | token、步数、时间预算 | 凭据按租户 | 轨迹回放
```
层间规则：
- 上层只通过任务和工具接口调用下层，不跨层直连数据库或浏览器。
- 数据访问统一走 repository 层，所有查询强制带 tenant_id；换 PostgreSQL 时只改这一层。
- 模型只做规划、重定位建议、失败归因、用例生成；点击、填写、断言、发请求、发压全部由代码执行。

### 2. 端到端数据流（以 UI 用例为例，接口、性能同构）
1. 入口：用户在控制面选用例（自然语言场景 + URL，或录制步骤，或 Excel）。入口层只校验一次（租户、权限、URL 白名单、必填项、预算配额），内部不再重复校验。写 tasks 表（tenant_id、type、档位配置、预算）并入队。
2. 出队：worker 取任务，建 run 记录，按租户加载凭据（运行时注入，不进日志、不进 prompt 以外的任何落盘内容），创建 Deep Agents 实例，thread_id = run_id（checkpoint 与回放用）。
3. 规划：Supervisor 按 type 路由到子 Agent。先从 Chroma 与关系库取上下文（需求文档、历史失败模式、已批准的定位器），再由 Planner 输出结构化步骤 [{action, target, role, value, description}]；步骤校验沿用 auto_ai 的 validate_steps。
4. 执行（确定性）：逐步调工具 navigate / click_smart / fill_smart / wait / assert。感知 = 精简快照 + 指纹缓存；定位 = 5 级降级链（0 token）；断言 = 代码优先，LLM 兜底。每步写 step_results：状态、定位策略链、耗时、API 追踪、截图路径。
5. 失败分支：进入自愈（第 5 节）。自愈失败则归因为「脚本漂移」或「产品缺陷」；crash（浏览器或页面不可恢复）立即终止，不在坏页面上继续报错。
6. 汇总：Reporter 生成报告（步骤、证据、token 用量、降级记录），写 reports 目录与数据库，通知控制面。
7. 回写：自愈补丁与新定位器只进「待审批」，人工批准后才写入定位器库；批准结果与操作人写审计日志。
8. 沉淀：失败模式、自愈历史写关系库；需求文档、OpenAPI、历史缺陷按需向量化写入 Chroma。

## 需求变更：部署后提供 MCP 服务（2026-11-21）
- [2026-11-21] 用户要求：平台部署后创建 MCP server，供各大 coding agent（Claude Code、Codex、Cursor、OpenCode 等）调用，用于软件开发的测试阶段。
- 设计含义：MCP 是控制面的另一个入口，与 Web 走同一套入口校验、租户、RBAC、预算、审计，不绕过任何治理规则。
- 工具草案：create_test_task、run_task、get_run_status、get_run_report、list_cases、generate_cases_from_openapi、run_perf_scenario、list_pending_patches（只读）。长任务异步，立即返回 run_id，由调用方轮询状态与报告。
- 约束：批准自愈补丁、删除用例等写操作不通过 MCP 开放，必须人工在控制面完成；API key 绑定租户与预算；调用同样写审计日志。
- 待用户确认：(1) 传输方式（stdio 还是 streamable HTTP，企业多租户倾向 HTTP）；(2) 鉴权方式；(3) 首批开放哪些工具。

### 3. 各模块处理
1. 控制面（record-studio 改造）：只做入口校验、任务编排与展示。校验项：租户、RBAC、URL 白名单、必填项、预算配额，只在这一层校验一次。写 tasks 表后入队，不直接调用浏览器。现有的用例、套件、并发、调度器保留，补 tenant_id、登录、审计、审批中心。
2. 任务队列与 worker：先用进程内队列，接口抽象成 enqueue、dequeue、ack，后续换 Celery 或 Redis 不改调用方。每个 run 独立浏览器上下文，租户并发数受配额限制，超时和崩溃由 worker 回收进程。
3. Supervisor：只做按 type 路由与预算分配，不执行具体步骤。路由到 UI、接口、性能三个子 Agent，子 Agent 之间不互相对话，只通过 state 交接结果。
4. UI 子 Agent（迁移自 auto_ai/agent.py）：Planner 出结构化步骤，Executor 逐步调工具，Reflect 节点只在失败时触发且次数有上限。感知用精简快照加指纹缓存，定位用 5 级降级链，断言代码优先、LLM 兜底，crash 立即终止。
5. 接口子 Agent（新建）：输入为 OpenAPI、抓包或自然语言。Planner 生成请求序列、参数化与依赖提取；Executor 用 HTTP 客户端发请求；断言为状态码、JSON Schema、字段值、耗时，全部由代码判定；LLM 只做用例生成与失败归因。鉴权信息运行时按租户注入。
6. 性能子 Agent（新建）：LLM 只生成场景配置（并发、梯度、时长、思考时间、检查点），由 Locust 或 k6 在受控 worker 上发压；压测目标必须在租户白名单内，并设置并发和时长上限。LLM 读取聚合指标（P95、错误率、吞吐）给出瓶颈假设，不直接产生负载。
7. 浏览器驱动接口：Playwright 为基线实现，支持企业指定的浏览器通道；ego (lite) 为可选实现，子进程调用 ego-browser nodejs，仅在本机或带桌面的执行机启用。回放只用稳定选择器，不保存 @N。
8. 决策接口：沿用 choose(state, criteria, meta)，返回候选与校验过的概率。laya、jev 为可选实现，基线可不挂；概率缺失或无效时不使用猜测分数，失败时明确降级并记录原因。
9. 记忆与知识：短期记忆为 LangGraph state 加 checkpoint（thread_id = run_id）；长期记忆为定位器库、自愈历史、失败模式（关系库）；Chroma 存需求文档、OpenAPI、历史缺陷，检索结果必须带 tenant_id 过滤；Neo4j 保持可选。
10. 报告与证据：每步记录状态、定位策略链、耗时、API 追踪、截图、token 用量、降级记录。报告按租户隔离存储，轨迹可回放。
11. 治理：审计日志记录谁在何时对什么做了什么（含 MCP 调用与审批）；token、步数、时间预算按租户与任务双重限制；凭据按租户加密保存，不进日志与 prompt 之外的任何落盘内容。

### 4. 数据模型（SQLite，所有表带 tenant_id，经 repository 层访问）
| 表 | 关键字段 | 说明 |
|---|---|---|
| tenants | id, name, plan, quota_json | 租户、套餐与配额（并发数、token、任务数） |
| users / roles | id, tenant_id, role | RBAC：管理员、测试工程师、只读 |
| projects | id, tenant_id, base_url, allowed_hosts | URL 白名单在此维护 |
| credentials | id, tenant_id, project_id, kind, ciphertext | 加密保存，运行时注入，不出现在日志 |
| cases | id, tenant_id, project_id, type(ui/api/perf), source, content_json, version | 用例；UI 步骤只存稳定选择器，不存 @N |
| suites | id, tenant_id, case_ids, schedule | 套件与调度 |
| tasks | id, tenant_id, type, tier_config, budget_json, status, created_by, channel(web/mcp) | 入队的任务 |
| runs | id, task_id, tenant_id, thread_id, status, started_at, ended_at, token_usage | 一次执行；thread_id 对应 checkpoint |
| step_results | id, run_id, tenant_id, seq, action, status, locate_chain, api_trace, screenshot, duration_ms | 每步证据 |
| locators | id, tenant_id, project_id, page_pattern, role, name, selector, status(approved/pending), version | 定位器库，只有 approved 参与回放 |
| heal_patches | id, tenant_id, run_id, step_seq, old_selector, new_selector, verdict(drift/defect), status(pending/approved/rejected), reviewer | 自愈补丁，待审批 |
| failure_patterns | id, tenant_id, signature, cause, fix_hint, count | 失败模式沉淀，供归因检索 |
| perf_results | id, run_id, tenant_id, p50, p95, p99, error_rate, rps, hypothesis | 性能指标与瓶颈假设 |
| fallback_events | id, run_id, tenant_id, provider, reason, at | 增强档降级记录 |
| audit_logs | id, tenant_id, actor, channel, action, target, at | 审计，含 MCP 调用与审批 |

数据流转要点：
- 写入顺序：tasks -> runs -> step_results；失败时再写 heal_patches。
- locators 只由人工批准后更新；heal_patches 批准的同一事务内写 locators 并写 audit_logs。
- Chroma 的 collection 按租户命名，检索时再带 tenant_id 过滤，双重隔离。
- 文件（截图、trace、报告）放 data/{tenant_id}/{run_id}/，数据库只存相对路径。

### 5. 自愈与审批
```
步骤失败
  -> L1 代码降级链（role -> 精确文本 -> 模糊文本 -> 滚动重试），0 token
  -> L2 定位器库历史选择器与 DOM 差异比对，0 token
  -> L3 LLM 重定位（只给最新精简快照，最多 1 次；增强档可用 laya / jev 做候选选择）
  -> 用新定位重试一次，由页面断言验证，模型选择本身不算通过
  -> 判别：定位变了但功能正常 = 脚本漂移；功能行为或 API 响应异常 = 产品缺陷
```
- 脚本漂移：生成 heal_patches（pending），本次 run 标记为「已自愈」，不改用例；人工在审批中心批准后才写入 locators。
- 产品缺陷：run 标记失败，附截图、API 错误、页面快照，进入缺陷列表，不自愈。
- 自愈只试一次，不循环；同一步连续多次漂移时升级为人工复核。
- 判别依据以代码可验证的信号为主（API 状态码、页面断言、DOM 结构差异），LLM 只给归因说明，不单独决定缺陷与否。

### 6. 降级与预算
- 增强档（ego、laya、jev）不可用、超时或报错：按配置降级到基线，写 fallback_events，并在报告顶部列出，不静默回退。
- 预算：每个 run 有步数、token、时间三个上限；任一超限立即终止并标记 budget_exceeded；租户级配额在入口检查。
- LLM 调用失败：重试次数有上限，仍失败则该步标记 error，不让整个 run 无限卡住。
- 并发：租户并发数、浏览器进程数、压测并发数分别限额。

### 7. MCP 服务（部署后提供给 coding agent，用于开发的测试阶段）
```
Coding agent（Claude Code / Codex / Cursor / OpenCode ...）
  | MCP 调用（API key，绑定 tenant_id）
MCP server（控制面的另一个入口，Python，与 FastAPI 同进程或同仓库）
  | 入口校验（租户、RBAC、URL 白名单、预算配额），与 Web 完全同一套
任务队列 -> Agent 层 -> 工具层 -> 报告
  | run_id
调用方轮询 get_run_status / get_run_report
```
- 传输方式：企业多租户倾向 streamable HTTP（部署在服务端，一个地址多个 agent 共用）；stdio 只给本地开发调试用。待用户确认。
- 鉴权：API key 绑定租户、角色、预算；key 可吊销、可设有效期；只存哈希。每次调用写 audit_logs，channel = mcp。
- 工具清单（首批草案，待用户确认）：
  - create_test_task(type, project_id, scenario | case_ids, tier)：创建任务，立即返回 task_id 与 run_id。
  - run_task(task_id)：触发执行，异步。
  - get_run_status(run_id)：状态、进度、已用 token 与步数。
  - get_run_report(run_id)：摘要加失败步骤、证据路径、降级记录、缺陷归因。
  - list_cases(project_id, filter)：列出用例。
  - generate_cases_from_openapi(project_id, spec_url | spec_text)：生成接口用例草稿，状态为 draft，需人工确认后才启用。
  - run_perf_scenario(project_id, scenario)：提交压测场景，受白名单与并发时长上限约束。
  - list_pending_patches(project_id)：只读，列出待审批的自愈补丁。
- 明确不开放：批准或拒绝自愈补丁、删除用例、修改凭据、修改白名单与配额，这些只能在控制面由人工完成。
- 典型流程：coding agent 改完代码并部署到测试环境 -> create_test_task 与 run_task -> 轮询 get_run_status -> get_run_report -> 失败则把缺陷归因与证据反馈给 coding agent 修复 -> 再跑一次。产品缺陷与脚本漂移在报告里分开列，避免 coding agent 去「修」其实只是选择器变了的问题。
- 安全约束：长任务不阻塞 MCP 调用；返回内容不含凭据与完整快照，只给证据路径与摘要；目标 URL 不在项目白名单内一律拒绝；单个 key 有调用频率限制。

### 8. 目录落位与迁移步骤
| 现有位置 | 去向 | 处理 |
|---|---|---|
| record-studio/backend | platform/control（控制面） | 补 tenant_id、登录、RBAC、审计、审批中心；先备份再清理 28 个下划线开头的临时文件（目前不是 git 仓库，删除不可恢复） |
| record-studio/frontend | platform/web | 补租户切换、审批中心、报告页 |
| auto_ai/browser.py、snapshot.py | engine/ui/drivers | 抽成浏览器驱动接口，Playwright 为基线实现 |
| auto_ai/agent.py（953 行） | agents/ui | Planner、Executor、Reflect 迁到 Deep Agents，确定性逻辑（降级链、断言链）原样保留为工具 |
| auto_ai/knowledge_graph.py | engine/knowledge（可选） | 放到 repository 接口后面，默认关闭 |
| 新建 | agents/api、agents/perf、engine/api、engine/perf | 接口与性能子 Agent 及其工具 |
| 新建 | platform/mcp | MCP server |
| 新建 | engine/providers | ego 驱动、laya / jev 决策接口（增强档） |
| lemon-TestPilot-main、laya-pilot-main | 保持原位，只参考 | 不合并代码 |

迁移顺序（每步可验证）：
1. Spike：Deep Agents 跑登录场景，与原实现对比 token、自愈成功率、可复现性。
2. 抽浏览器驱动接口，原测试全部通过。
3. UI 子 Agent 迁到 Deep Agents，原测试全部通过。
4. repository 层加 tenant_id，补登录、RBAC、审计。
5. 自愈补丁与审批流程。
6. 接口子 Agent，再性能子 Agent。
7. MCP server。
8. 增强档 provider（ego、laya、jev）。

## 进度记录（2026-11-21）
- [x] auto_ai/config.py 的 NEO4J_PASSWORD 默认值已清空。
- [x] 架构定稿第 1 到 8 节已写入，含分层、数据流、各模块处理、数据模型、自愈与审批、降级与预算、MCP 服务、目录落位与迁移步骤。
- [~] Spike 第 1 步部分完成：spike_deepagents/.venv 已安装 deepagents 0.7.41、langgraph 1.2.24、langchain-openai 1.8.5（Python 3.13.7）；尚未用 DeepSeek 接口跑通最小示例。
- 风险：record-studio/backend 有 28 个下划线开头的临时文件（约 88 KB），目录不是 git 仓库，清理前先备份并列清单给用户确认。
- 阻塞：.env 现有变量只有 LLM 与 Neo4j，没有测试站点 https://api.toporeduce.cn 的账号变量，需用户确认变量名后才能跑登录场景。
- 整理项：「需求变更：部署后提供 MCP 服务」位于架构第 2、3 节之间，后续整理时移入需求变更区；MCP 细节以第 7 节为准。
- MCP 传输方式、鉴权方式、首批工具已由助手决定见第 10 节，可改。

## 需求变更：MCP 按代码生成测试并反馈报告与修复方案（2026-11-21）
- [2026-11-21] 用户要求：MCP 根据 coding agent 传入的代码内容，使用 coding agent 本地接入的模型生成功能测试用例、接口测试用例、UI 自动化测试用例，平台执行后把测试报告、bug 和具体修复方案反馈给 coding agent。
- 补充第 7 节：MCP 不只是触发已有用例，还要完成「代码变更 -> 生成用例 -> 执行 -> 归因 -> 修复建议 -> 重测」的闭环。
- 与已定原则的关系：模型只负责生成用例、归因和修复建议；执行、断言、判定仍由确定性代码完成。平台不修改被测代码，修复由 coding agent 执行。

### 9. MCP 代码驱动的测试闭环（补充第 7 节）
```
coding agent 提交代码变更
  -> 入口校验 + 敏感信息扫描
  -> 变更影响分析（确定性代码）
  -> 用例生成（优先 coding agent 的本地模型）
  -> 用例校验（schema），状态 draft，临时运行
  -> 确定性执行（UI / 接口 / 功能）
  -> 失败分类 + bug 报告 + 修复建议
  -> 结构化报告回传 -> coding agent 修复 -> retest
```
1. 提交：submit_code_change(project_id, diff 或文件内容, 变更说明, 被测环境地址)。入口一次校验：租户、项目、URL 白名单、代码大小上限；扫描密钥与口令特征，命中则拒绝或脱敏，不带进后续流程。
2. 变更影响分析（代码，不用模型）：解析 diff，提取受影响的路由、接口、页面组件、函数，输出影响清单，并关联已有用例，已有用例能覆盖的不重复生成。
3. 用例生成，三种方式按配置优先级尝试，并记录实际用了哪种：
   - A. MCP sampling：平台作为 MCP server 向 coding agent 客户端发起 sampling 请求，使用其本地接入的模型。依赖客户端支持 sampling，通常需用户逐次批准。各 coding agent 的支持情况待核实。
   - B. 任务包：平台返回生成任务包（影响清单、相关代码片段、输出 schema、提示词），由 coding agent 用自己的模型生成，再用 submit_generated_cases 回传。
   - C. 兜底：使用租户配置的平台 LLM（如 DeepSeek 的 OpenAI 兼容接口）。
4. 生成结果校验：按 schema 校验一次，分三类：functional（落成接口或 UI 步骤）、api、ui。非法条目丢弃并记录原因。通过校验的状态为 draft，只在本次临时运行里执行，不进入用例库；转正需人工在控制面确认。
5. 执行：与第 2 节数据流一致。UI 走浏览器驱动接口，接口走 HTTP 客户端，全部确定性断言，预算与降级规则不变。
6. 失败分类，先分四类：脚本漂移、用例错误、产品缺陷、环境问题。用例的期望值来源要标注：需求文档、OpenAPI、代码推断。只靠代码推断的期望值失败时，默认标 needs_review，不直接判为产品缺陷，避免模型推断错误造成误报。
7. bug 报告与修复建议：输入为 diff、失败证据（API 追踪、截图、快照、日志）和相关代码片段，输出根因、可疑文件与位置、修复方案、验证用例、置信度。修复方案是建议，不是结论，由 coding agent 判断是否采纳。生成方式同第 3 步 A、B、C。
8. 回传：get_run_report 返回结构化结果。
   - summary：通过、失败、跳过数量，所用生成方式，降级记录。
   - bugs[]：id、title、severity、steps_to_reproduce、expected、actual、expected_source、evidence、suspected_location、fix_suggestion、confidence。
   - drift_items[]：脚本漂移项，单独列出，提示 coding agent 不要去改业务代码。
   - needs_review[]：期望值来源不可靠的失败项。
9. 重测：coding agent 修复后调用 retest(run_id, bug_ids)，只重跑相关用例。单个变更最多重测 3 轮，超限转人工，防止无限循环；每轮计入预算。

新增工具（补充第 7 节清单）：submit_code_change、get_generation_task、submit_generated_cases、list_bugs、get_bug_detail、retest。

约束与风险：
- 源码是敏感数据：默认只保存 diff 哈希和影响清单，不持久化源码；运行期间的临时副本按租户保留期删除；不写日志，不进 Chroma，除非项目显式开启。
- 方式 A、B 下源码留在 coding agent 一侧的模型里，方式 C 会把代码片段发给平台 LLM，需要租户明确同意并受数据出境规则约束。
- 模型生成的用例覆盖面不保证。用例只算草稿，报告里必须写明是自动生成的，且生成方式与来源可追溯。
- 本流程只发现问题和给建议，不自动合并、不改被测系统，也不通过 MCP 开放审批补丁、删除用例、改凭据与配额。

待用户确认：
1. 首批要支持的 coding agent 及其 sampling 支持情况，由我去核实官方文档。
2. 提交范围：只提交 diff，还是允许提交整个文件或整个仓库。
3. 被测环境由谁部署：coding agent 部署后把地址传给平台，还是平台侧配置固定的测试环境。
4. 源码临时副本的保留期。
5. 平台 LLM 兜底（方式 C）是否允许，默认开启还是默认关闭。

## 用户确认（2026-11-21，MCP 用例生成方式）
- [2026-11-21] 用户确认：第 9 节用例生成以方式 A 为主，即 MCP sampling，借用 coding agent 本地接入的模型。此前单独回复的「A」按此理解，用户已重复确认。
- 含义：平台作为 MCP server 向客户端发起 sampling 请求，由客户端本地模型生成功能、接口、UI 用例。生成环节平台不持有模型凭据，源码留在 coding agent 一侧。
- 约束：sampling 依赖客户端支持，通常需用户逐次批准。Claude Code、Codex、Cursor、OpenCode 的支持情况尚未核实，不得假设。
- 未决：客户端不支持 sampling 时怎么办（方式 B 任务包、方式 C 平台 LLM、或直接报错），用户未指定。暂按「不自动启用方式 C」处理，因为 C 会把代码片段发给平台 LLM，需租户明确同意。
- 不变：生成结果仍按第 9 节第 4 步做 schema 校验，状态为 draft，只在本次临时运行里执行，不进用例库。每条用例记录 generation_mode，取值 sampling、task_pack、platform_llm。
- 待办：[x] 查官方文档，核实各 coding agent 对 MCP sampling 的支持情况（2026-11-21 已完成，结果见「sampling 支持核实」一节）。
- 待用户确认：(1) sampling 不可用时的处理；(2) 首批支持的 coding agent；(3) 提交范围（只收 diff 还是允许整个文件或仓库）；(4) 被测环境由谁部署；(5) 源码临时副本保留期；(6) MCP 传输方式、鉴权方式、首批开放工具；(7) .env 里测试站点的账号变量名。

## sampling 支持核实（2026-11-21）
- 方法：读取各客户端官方 MCP 文档页面全文，搜索 sampling 一词。只读，未安装、未实测。
- 结果：Claude Code（code.claude.com/docs/en/mcp）、Codex（developers.openai.com/codex/mcp）、OpenCode（opencode.ai/docs/mcp-servers）、Cursor（cursor.com/docs/context/mcp）四个页面，sampling 命中均为 0。Cursor 的协议支持表列出 Tools、Prompts、Resources、Roots、Elicitation、Apps，没有 Sampling。Claude Code 与 OpenCode 的文档只提到 elicitation，那是向用户索取输入，不是借用客户端模型，两者不要混淆。
- 结论：官方文档均未见 sampling 支持。这只说明「文档未写」，不等于「一定不支持」，未经实测，不得写成「不支持」。
- 规范依据：MCP 规范要求支持 sampling 的客户端在初始化时声明 sampling 能力，并要求有人工审批环节。
- 设计影响：方式 A 在这四个客户端上大概率不可用，方式 B（任务包）可能成为实际主路径。平台 MCP server 在握手时读取客户端声明的 capabilities：声明了 sampling 就走 A，否则自动走 B，并在报告里记录 generation_mode 与原因，不静默切换。
- 待办：[ ] 写一个最小 MCP server，分别接入 Claude Code、Codex、Cursor、OpenCode，实测它们是否声明 sampling，把实测结果替换本节结论。
- 待用户确认：(1) 方式 B 作为 sampling 不可用时的默认路径，是否接受；(2) 是否先做最小 MCP server 实测，再决定第 9 节的细节。

## 需求变更：平台扩展为完整测试管理闭环（2026-11-21）
- [2026-11-21] 用户要求：平台自身要支持完整测试流程，不只是 MCP 触发。需求录入 -> 自动生成用例 -> 人工审核修改 -> 定稿 -> 执行 -> 提交 bug -> bug 分级 -> 复测 -> 关闭或重新打开；内置 bug 标识管理；按代码版本做需求影响范围分析，生成该版本增量用例并并入环境用例库，也可手动选用例执行；输出专业测试报告（bug 图表、用例执行情况、环比同比等趋势）；支持白盒测试。
- 代码来源两种：(1) coding agent 经 MCP 提交；(2) 平台侧自行选定代码并创建测试环境。
- 首批客户端：Claude Desktop、Codex、Trae、Cursor、WorkBuddy 等。sampling 支持未核实：此前只查过 Claude Code、Codex、OpenCode、Cursor 的文档，均未见；Trae、WorkBuddy、Claude Desktop 还没查。
- 源码平台不保留：只存 diff 哈希和影响清单；执行期间的临时工作区在 run 结束后立即删除，不写日志、不进 Chroma、不进报告。
- 「提交范围」指 coding agent 经 MCP 提交代码时传 diff 还是整文件或整仓库；用户反问后确认范围随代码来源而定，见下方决策。

### 10. MCP 决策（用户授权由助手决定，可改，待确认）
- 传输：streamable HTTP，部署在服务端，多个 agent 共用一个地址；stdio 只用于本地调试，工具集相同。
- 鉴权：API key，绑定租户、项目、角色与预算；只存哈希；可吊销、有有效期、有调用频率限制；每次调用写 audit_logs（channel=mcp）。OAuth 不进首批。
- 提交范围（MCP 来源）：以 diff 为主，附被改文件内容和必要的上下文文件（路由定义、调用方），设大小上限，不接收整仓库。需要完整源码的白盒测试走平台自选来源，不走 MCP。
- 生成方式：握手时读取客户端声明的 capabilities，声明 sampling 走方式 A，否则走方式 B（任务包），报告记录 generation_mode 与原因，不静默切换；方式 C 默认关闭。
- 首批工具：submit_code_change、get_generation_task、submit_generated_cases、run_task、get_run_status、get_run_report、list_bugs、get_bug_detail、mark_bug_fixed（只能置为已修复并附说明）、retest。
- 不开放：用例审核定稿、关闭 bug、批准自愈补丁、删除用例、改凭据白名单配额，只能人工在控制面完成。

### 11. 用例与 bug 生命周期
- 用例：draft -> in_review -> approved（定稿）-> deprecated。定稿后再改产生新版本，执行记录绑定用例版本号。每条用例标注来源与期望值来源（需求、OpenAPI、代码推断）；只靠代码推断的不能直接定稿为缺陷依据。
- 版本与影响：versions 记录 commit 或 tag 与对应需求变更；影响分析输出受影响的需求、接口、页面，再关联已有用例；覆盖不到的部分生成增量用例（draft），审核定稿后并入该环境用例库。执行范围可选：全量、增量、手选、按 bug 复测。
- bug 状态：新建 -> 已确认 -> 修复中 -> 已修复（待复测）-> 复测通过则关闭，复测失败则重新打开；另有已拒绝（非缺陷）和重复。关闭只能由复测通过或人工完成，coding agent 不能直接关闭。
- bug 标识：BUG-{项目代号}-{序号}，项目内唯一且不复用；同时保存失败签名，复测失败自动关联原 bug 并重新打开，不新建。
- bug 分级：严重程度 S1 致命（主流程中断、数据丢失）、S2 严重、S3 一般、S4 轻微；优先级 P0 到 P3 单独记录。系统按规则给建议级别（主流程用例失败、5xx、数据错误等），人工可调，调整留痕。

### 12. 白盒测试（范围未定，不假设）
- 用户只说「平台还支持白盒测试」，没有给范围。候选：单元测试生成与执行、覆盖率统计、静态分析、分支与路径分析。
- 与「源码不保留」有张力：白盒需要在运行期间读取完整源码，只能放在临时工作区，run 结束即删；覆盖率与分析结果只存指标，不存源码片段。
- 待用户确认：(1) 首批白盒做哪几类，涉及哪些语言；(2) 源码在运行期间可否进入平台自己的工作区。

### 13. 数据模型扩展（接第 4 节，所有表带 tenant_id）
| 表 | 关键字段 | 说明 |
|---|---|---|
| requirements | id, tenant_id, project_id, code, title, content, status, version_id | 需求录入，code 为需求标识，项目内唯一 |
| versions | id, tenant_id, project_id, name, commit_or_tag, diff_hash, impact_json, created_at | 代码版本；只存 diff 哈希与影响清单，不存源码 |
| version_requirements | version_id, requirement_id | 版本与需求的影响关系 |
| environments | id, tenant_id, project_id, name, base_url, source(mcp/platform), status | 测试环境；用例库按环境归属 |
| case_versions | id, case_id, tenant_id, version_no, status(draft/in_review/approved/deprecated), expected_source, generation_mode, reviewer, approved_at | 用例版本；执行记录绑定 version_no |
| case_requirements | case_id, requirement_id | 用例与需求的覆盖关系，用于覆盖率统计 |
| env_cases | environment_id, case_id, case_version_no | 并入某环境的定稿用例 |
| bugs | id, tenant_id, project_id, bug_code, title, severity(S1-S4), priority(P0-P3), status, signature, reporter, found_in_version, fixed_in_version, closed_by, reopen_count | bug_code 形如 BUG-{项目代号}-{序号}，不复用 |
| bug_events | id, bug_id, tenant_id, from_status, to_status, actor, channel, note, at | 状态流转与分级调整留痕 |
| bug_runs | bug_id, run_id, step_seq | bug 与出现它的执行关联；复测失败时追加关联 |
| whitebox_results | id, run_id, tenant_id, kind, metrics_json | 白盒结果，只存指标，不存源码片段（范围待确认） |
| report_snapshots | id, tenant_id, project_id, period, metrics_json, created_at | 按周期固化的指标，保证同比环比口径稳定 |

数据流转要点：
- 失败步骤先算失败签名（接口路径+状态码+断言类型，或页面模式+定位器），匹配未关闭 bug 则追加 bug_runs，已关闭则重新打开并 reopen_count 加一，否则新建。
- 状态变更一律写 bug_events，同一事务内更新 bugs。
- 临时工作区路径只存在于运行期内存和临时目录，不入库。

### 14. 测试报告与趋势指标
报告类型：单次执行报告、版本报告、周期报告（周、月、迭代）。均按租户隔离，可导出 PDF 与 HTML。

单次执行报告：用例总数、通过、失败、跳过、needs_review、自愈次数；耗时；token 用量；增强档降级记录；bug 列表（含级别与失败证据路径）；自动生成用例的数量与来源占比，写明 generation_mode。

版本报告：版本的需求影响范围；受影响需求的用例覆盖率；增量用例数、并入环境用例库的数量；本版本新增 bug、复测关闭、重新打开；与上一版本的对比。

指标口径（先定义，报表再实现）：
- 通过率 = 通过 / (通过+失败)，不含跳过与 needs_review；needs_review 单列。
- 需求覆盖率 = 至少有一条 approved 用例关联的需求数 / 需求总数。
- 缺陷密度 = 新增 bug 数 / 本版本影响的需求数。
- 缺陷逃逸 = 上线后发现的 bug 数 / 总 bug 数；需要标注 bug 发现阶段，字段待补。
- 复开率 = reopen_count 大于 0 的 bug 数 / 已关闭 bug 总数。
- 平均修复时长 = 从已确认到已修复的时间均值，复测关闭时长单列。
- 环比 = 本周期值 / 上一周期值 - 1；同比 = 本周期值 / 去年同周期值 - 1。分母为 0 时显示 N/A，不显示 0 或无穷。
- 同比需要至少一年历史，数据不足时明确显示数据不足；周期指标固化到 report_snapshots，之后用例或 bug 数据变化不改已固化的历史值。

图表：bug 严重程度分布、bug 状态分布、按版本的新增与关闭趋势、用例执行结果趋势、通过率趋势、需求覆盖率、缺陷密度、复开率、平均修复时长，另有 Top 失败用例与 Top 不稳定用例。

待用户确认：(1) 报告导出格式是否需要 Excel；(2) 缺陷发现阶段要不要区分（开发自测、测试、上线后）；(3) 严重程度 S1 到 S4 的判定细则是否使用企业已有规范。

### 15. 测试环境创建与代码版本
- 环境来源两种，都落 environments 表，用例库按环境归属：
  1. MCP 提交：coding agent 传 diff 与被改文件，平台做影响分析后，在该项目已登记的环境地址上执行，不自行部署代码。地址由项目配置或调用方按白名单传入，平台不信任任意 URL。
  2. 平台选定：用户选代码版本并创建环境。这是「源码不保留」和「白盒测试」最容易冲突的地方，需要用户明确。
- 建议（待确认）：平台侧环境创建只做到「接管已部署环境」（登记 base_url 与凭据），不做自动构建和部署。若确需自动构建，需要额外决策：由谁提供构建机、源码在临时工作区停留多久、构建日志如何脱敏。不在用户确认前实现。
- 版本与执行绑定：每次执行记录 version_id；对同一版本重复执行时复用 version_id，不重复存源码。
- 环境隔离：租户之间的环境地址、凭据、用例库、报告完全隔离；URL 白名单按项目维护，端口与路径可限定。
- 环境健康检查：执行前探测 base_url 可达性，不可达直接标记 environment_error，不消耗 LLM 预算，不误判为产品缺陷。

### 16. 仍待确认事项汇总
阻塞 Spike 的：
1. `.env` 里测试站点 https://api.topreduce.cn 的账号变量名（我只读变量名，不读值）。

阻塞功能设计的：
2. 白盒测试首批范围与语言，源码能否在运行期进入平台临时工作区。
3. 平台侧测试环境创建：只接管已部署环境，还是需要平台自动构建与部署（涉及构建机与源码停留策略）。
4. 首批客户端里 Trae、WorkBuddy、Claude Desktop 的 sampling 支持需实测；方式 B 作为默认回退是否接受。
5. 报告导出是否需要 Excel；缺陷发现阶段是否区分；S1 到 S4 判定细则是否沿用企业规范。
6. 增强档启用粒度（租户套餐、项目、任务、失败后升档）；jev 接口形态与文档。
7. record-studio/backend 下 28 个下划线开头临时文件的清理（先备份再删，需同意）。

已由助手决定、可随时修改：
8. MCP 传输用 streamable HTTP，鉴权用绑定租户的 API key，首批工具清单见第 10 节。

## 需求变更：平台提供可安装的 exe / dmg（2026-11-21）
- [2026-11-21] 用户要求：最终平台要打包成可安装的 exe（Windows）与 dmg（macOS）。
- 冲突点（必须先定，否则打包方式完全不同）：平台此前定为「企业多租户服务端」。多租户 SaaS 通常只部署在服务器，浏览器打开即用，不需要客户端安装包；而 exe / dmg 是单机应用形态。两者不是同一个东西。
- 三种解释，待用户选择：
  - A. 单机版：整个平台打包成一个应用，每台电脑一份，SQLite 本地库，多租户退化为多用户 / 多项目。交付简单，但企业集中管理和数据汇总困难。
  - B. 双形态：服务端 SaaS（多租户）+ 桌面执行端（exe / dmg）。桌面端负责本机浏览器执行，装 ego (lite)、跑有头浏览器、连企业内网；服务端负责用例、调度、报告、审批。ego (lite) 只放在本机这一档，与已确认的 ego 定位一致。
  - C. 只打包执行端：exe / dmg 仅为 runner，服务端仍是 Web。本质与 B 相同，只是不叫平台客户端。
- 技术约束（无论选哪种）：
  - dmg 必须在 macOS 上构建，Windows 无法交叉编译；exe 可在 Windows 构建。需要 macOS 构建机或 CI（如 GitHub Actions 的 macos runner）。
  - 打包工具：PyInstaller 或 Nuitka；Playwright 浏览器二进制体积大（每平台约数百 MB），首次运行需下载或随包分发，需要单独决策。
  - record-studio 前端是 Vue 3 + Vite，需先构建成静态资源再打入安装包。
  - 代码签名：Windows 需 EV 证书，macOS 需 Apple Developer ID 与公证（notarization），否则用户安装时会被拦截。这是企业交付的硬门槛，需确认是否有证书。
- 待用户确认：(1) 选 A、B 还是 C；(2) 是否有 macOS 构建机与代码签名证书；(3) Playwright 浏览器二进制随包还是首次下载。

## 进度记录（2026-11-21，续）
- [x] .env 已补 TEST_BASE_URL、TEST_USERNAME、TEST_PASSWORD（2026-11-21，用户写入，未读取值）。
- [x] deepagents 0.7.41 已装在 spike_deepagents/.venv，无需 Docker 镜像；本机 Docker 守护进程未运行。
- 未做：Spike 第 2 到 5 步（封装工具、跑登录场景、与原实现对比）。

## Spike 实测记录（2026-11-21）
- 环境：spike_deepagents/.venv，Python 3.13.7，deepagents 0.7.41、langgraph 1.2.24、langchain-openai 1.8.5、playwright、python-dotenv、neo4j、openpyxl；Chromium 已装。
- 脚本：spike_deepagents/run_login.py，把 BrowserSession 的 navigate、snapshot、click、fill、fill_password、wait_text、current_url、screenshot 包成 Deep Agents 工具，recursion_limit=40，记录耗时、模型轮次、浏览器操作次数、token。
- 凭据处理：密码不进 prompt。fill_password 工具内部从 .env 读取，模型只拿到「已填入密码」。这就是第 3 节要求的运行时注入。
- 阻塞：.env 里的 OPENAI_API_KEY（结尾 ac93）对 https://api.deepseek.com 返回 401，对 https://api.toporeduce.cn/v1 也返回 401。key 已失效或不属于这两处，需要用户换一个有效 key。
- 已修 bug 1：系统环境里存在另一个 OPENAI_API_KEY（结尾 3ff0），load_dotenv 默认不覆盖，导致实际用了错误 key。改为 dotenv_values 直接读 .env。平台上线后必须显式指定配置来源，不能让宿主环境变量串入。
- 已修 bug 2：BrowserSession.navigate 返回 (快照, 是否缓存) 元组，原工具直接把元组交给模型；已改为格式化字符串。
- 已验证：Playwright 能打开 https://api.toporeduce.cn，页面为 SPA，有 Sign in 链接与按钮、System Notice 弹窗（Close Notice / Close Today）、中英文导航。快照约 2000 字符，可正常用于定位。
- 未验证：Deep Agents 的模型轮次、token 消耗、自愈成功率（等有效 key）。

## 用户确认（2026-11-21，交付形态）
- [2026-11-21] 用户选择 B：双形态。服务端 SaaS（多租户）负责用例、调度、报告、审批、缺陷管理、趋势指标；桌面执行端打 exe / dmg，负责本机浏览器执行、ego (lite) 增强、连企业内网与本地已登录会话。
- 与已有决策一致：ego 只在桌面端启用；Playwright 在服务端与桌面端都可用（服务端无头，桌面端可有头）。
- 职责边界：桌面端不持有业务数据的主副本，只做本地执行与临时缓存；用例、bug、报告的权威数据在服务端，桌面端离线时排队、联网后同步。
- 接入方式：桌面端用设备令牌（device token）注册到服务端，领取任务、回报结果；服务端不信任桌面端上报的任意 URL，目标地址仍走项目白名单校验。
- 技术路线（建议，待确认）：Tauri 作外壳 + Vue 3 前端 + Python sidecar（FastAPI，本地执行器） + PyInstaller 打包；SQLite 存本地队列与缓存。选 Tauri 而非 Electron，体积更小。
- 交付硬约束：dmg 必须在 macOS 构建，需 macos runner 或 Mac 构建机；Windows 用 EV 证书签名，macOS 用 Developer ID 加公证；签名缺失会导致安装被系统拦截。
- 待用户确认：(1) 是否有 Windows EV 证书与 Apple Developer ID，没有的话首版可否接受未签名的内部分发包；(2) Playwright 浏览器二进制随包分发（体积大、开箱即用）还是首次运行下载（包小、需联网）。
- 新增待办：[ ] 桌面执行端最小闭环：本地跑一条用例 -> 上报结果到服务端；[ ] 打包验证：Windows exe 本机构建 + macOS dmg 需构建机。

## 顺序纠正（2026-11-21）
- [2026-11-21] 用户指出：此前只 pip 安装并跑了一次脚本，不算整合；deepagents 源码没拉、代码没整合、架构没落成代码、UI 一行没有，此时谈测试是本末倒置。
- 已完成：vendor/deepagents 源码已拉取，HEAD 65d33b2（2026-11-21），Python 包位于 libs/deepagents。
- 实际缺口：platform/control（控制面后端）不存在；platform/web（控制台 UI）不存在；agents/ 与 engine/ 不存在；record-studio 仍是单租户录制工作台，没有租户、缺陷、报告、审批；auto_ai 未接入 Deep Agents。
- 落地顺序（以此为准，不再跳步）：
  1. 仓库骨架与依赖：vendor 源码以可编辑方式安装，建 platform/control、platform/web、agents、engine。
  2. 后端垂直切片：租户 -> 项目 -> 用例 -> 执行 -> 报告，FastAPI + SQLite，全表 tenant_id。
  3. 控制台 UI：Vue 3 + Vite，接第 2 步的接口，先跑通同一条垂直切片。
  4. UI 自动化子 Agent 接 Deep Agents，迁移 auto_ai 的确定性工具。
  5. 接口自动化子 Agent。
  6. 性能测试子 Agent。
  7. MCP server。
  8. 桌面执行端 exe / dmg（双形态）。
- 结构决定（沿用架构第 8 节）：新建 platform/ 目录承载控制面与控制台；record-studio 原地保留，作为录制能力的参考与后续集成来源，不做整目录搬迁，避免一次性大改。

## 代码落地进度（2026-11-21）
- 目录：platform/control（FastAPI 控制面）、platform/web（Vue 3 控制台）、vendor/deepagents（源码，HEAD 65d33b2）。
- 后端已实现并验证：POST/GET /api/tenants、/api/projects、/api/cases，POST /api/cases/{id}/run，GET /api/runs、/api/runs/{id}。SQLite 表 tenants、projects、cases、runs 全部带 tenant_id，查询强制按租户过滤。
- 验证结果：建租户 8682ae355f44、项目 7255a59e3625、用例 f2e887bc8230、执行 92229ef5758f 全部成功；另一个租户读项目返回 0 条；不带 X-Tenant-Id 返回 400。
- 前端已实现并验证：Vue 3 + Vite，dev 端口 5190，/api 代理到 8090；工作区进入页、项目 / 用例 / 执行三个视图；`npm run build` 通过（JS 72.9 kB，gzip 28.8 kB）。用 Playwright 实际打开并截图，项目列表来自后端真实数据。
- 修过的坑：Vite 默认只监听 ::1，已显式配置 host 127.0.0.1，否则 127.0.0.1 连不上。
- 当前限制：执行记录只有 queued 状态，没有 worker 真正执行；无登录鉴权（只有 X-Tenant-Id 头）；无缺陷、报告、审批、趋势页面；未接入 Deep Agents。
- 下一步：worker 消费 runs 队列 -> 调 UI 子 Agent（Deep Agents）-> 写回 passed/report -> 前端展示结果。

## 架构变更：后端改用 Go（2026-11-21）
- [2026-11-21] 用户要求：构建的应用后端用 Go 部署；UI 太简单，需补齐需求里的功能。
- 修订：此前「后端统一 Python」改为「控制面 Go + Agent 执行端 Python」。理由：Go 单二进制、内存占用低、交叉编译方便，适合多租户服务端与后续打包。
- 分工：Go（platform/server）负责 HTTP API、SQLite、租户隔离、用例/需求/版本/环境/缺陷/审批/趋势；Python（agents/）负责 Deep Agents 编排、Playwright、ego、发压，通过设备令牌调 Go 的 heartbeat 与 finish 接口领取任务、回报结果。数据权威副本在 Go 侧。
- 技术选择：net/http 标准库路由（Go 1.22+ 方法路由，无需框架）、modernc.org/sqlite（纯 Go 驱动，无需 CGO，交叉编译无障碍）、google/uuid。单文件构建产物 14.3 MB。
- 已实现接口：tenants（建/列）、overview、projects、requirements、versions、environments、cases（建/列/审核）、runs（建/列/详情）、heartbeat、finish、bugs（建/列/流转）、patches（列/审批）、trends。
- 租户隔离：所有业务接口强制 X-Tenant-Id，查询与写入均带 tenant_id；执行端用 X-Agent-Token=tenant:<id>。
- 缺陷机制：finish 时按失败签名 upsert —— 已有未关闭则关联，已关闭则自动置 reopened 且 reopen_count+1，否则新建，编号 BUG-{租户前4位}-{序号}。状态机 new→confirmed→fixing→fixed→verified→closed，closed 可回到 reopened；非法跳转返回 409。
- 已实现的 UI：概览（8 张指标卡 + 30 天执行柱图 + 缺陷分布）、项目、需求、版本（含影响范围标签）、环境（平台选定 / MCP 来源）、用例（草稿→待审核→已定稿，审核通过/退回、执行、详情抽屉）、执行记录（状态、自愈标记、token、耗时、报告抽屉）、缺陷（严重级别可调、按状态机流转）、自愈审批（批准/拒绝）。共 9 页，用 Playwright 逐页截图验证。
- 已验证：完整链路 需求→版本→环境→用例→提交审核→定稿→执行→执行端领取→回报失败→自动建缺陷→五级流转至关闭→非法跳转 409→概览与趋势统计正确。
- 仍缺：登录鉴权（只有租户头/设备令牌）、MCP server、Python Agent 执行端真实对接、桌面端 exe/dmg、白盒测试、趋势同比环比（当前只有近 30 天日维度）、报告导出。

## 完成度盘点（2026-11-21，实测）
- 用户质问「整个平台建设完了么」，并指出 LLM key 与 URL 应由租户在平台设置里填写，不应向用户要 .env。认可，此前方向有误。
- 代码现状（实测）：Go 控制面 25 个接口、一行代码量 867 行；前端 9 个视图；platform/mcp 不存在；agents/ 不存在；设置页不存在。
- 已完成（可运行）：租户、项目、需求、版本、环境、用例审核流、执行记录、缺陷状态机与复开、自愈补丁审批、概览与日趋势。链路已端到端验证，但执行回报是模拟调用，不是真实跑浏览器。
- 未完成，按优先级：
  1. 设置模块（缺失且最基础）：租户级 LLM 配置（provider、模型、base_url、API key 加密存储）、运行预算（步数/token/时长）、增强档开关（ego/laya/jev）、MCP API key 管理。接口与页面都没有。
  2. 登录与 RBAC：目前只有 X-Tenant-Id 头，任何拿到 ID 的人都能进；没有用户、密码、会话、角色。
  3. Python Agent 执行端：真实接入 Deep Agents + Playwright，消费 heartbeat 任务、上报 finish。这是「自愈」真正跑起来的前提。
  4. MCP server：submit_code_change、get_generation_task、submit_generated_cases、retest 等代码驱动闭环工具均未实现。
  5. 接口自动化子 Agent、性能测试子 Agent（Locust/k6）。
  6. 白盒测试。
  7. 桌面端 exe / dmg（双形态第二部分）。
  8. 专业报告导出（PDF/Excel）、同比环比（现仅日维度）、用例版本历史表、环境健康检查。
- 结论：平台处于「骨架可运行」阶段，约完成基础闭环，远未建设完成。
- 约定（新）：LLM 的 key、base_url、模型名一律存数据库、按租户配置、密钥加密落库；不得要求用户改 .env，也不得把密钥写进代码或日志。

## 需求变更：测试数据生成与存储（2026-11-22）
- [2026-11-22] 用户要求：平台加入测试数据生成、测试数据存储。
- 数据模型：`test_data_sets` 保存租户、项目、环境、数据集名称、字段规则和记录数；`test_data_records` 保存每行结构化 JSON。数据集与记录均带 `tenant_id`，不能跨租户读取。
- 字段类型：姓名、邮箱、手机号、数字、日期、UUID、文本、密码；支持示例/前缀。示例存在时按行号生成变体，否则使用平台内置生成器生成合成数据。
- 敏感数据：字段可标记为敏感；Web 列表和预览统一返回掩码，执行端只能通过 `X-Agent-Token` 获取原始记录。原始值不进入报告、审计或普通接口响应。
- 生成策略：首版使用确定性代码生成，不让 LLM 直接生产可执行数据；后续可增加模型生成草稿，但必须经过 schema 校验和人工确认。
- 执行关系：当前 API 已提供数据集列表、创建、按数量生成、脱敏记录预览、执行端原始记录读取。用例和 run 的 dataset_id/版本快照引用尚未接入，下一步补齐后才能在执行记录中显示具体数据行。
- 前端：新增“测试数据”页面，放在“环境”和“用例”之间；支持项目/环境绑定、动态字段规则、敏感字段勾选、创建并生成、重新生成、数据预览。
- 验证：Go 接口真实生成 3 条记录；Web 端返回密码 `••••••`；执行端令牌返回原值；另一租户读取数据集与原始数据均返回 404；Vue 构建和 Playwright 页面截图通过。
- 首页概览：增加“测试数据集”和“测试数据记录”指标，并增加最近测试数据资产表，展示数据集、项目、记录数、字段和更新时间；首页不展示敏感值。
- 2026-11-22 复验：Go `go test ./...` 和 `go build` 通过；Vue `npm run build` 通过；Playwright 逐页截图通过，首页能显示“登录数据 / 3 条记录”，测试数据页面能显示生成表单和数据集。

## 后端功能推进（2026-11-22）
- 增量迁移：新增 `docs/migrations/2026-11-22-case-test-data-binding.md`。`cases` 与 `runs` 增加可空兼容的 `dataset_id`、`data_row_id`，旧数据库启动时自动补字段，不改已有数据。
- 执行绑定：创建用例/run 时校验数据集、项目、租户和数据行；heartbeat 返回固定的数据集/数据行和执行所需数据；跨租户绑定返回 400。真实链路已验证：case -> run -> heartbeat，数据字段正确返回。
- 租户设置：新增 `tenant_settings` 表和 `/api/settings`、`PUT /api/settings`、`/api/agent/settings`。普通设置接口不回显 API key，只返回 `api_key_configured`；服务端使用数据库旁的 `.key` 文件生成 AES-GCM 主密钥加密保存；执行端令牌接口才能取解密后的配置。
- Python 执行端：新增 `agents/ui_runner.py`，轮询 heartbeat，读取租户模型配置，使用 Deep Agents + Playwright 工具执行 UI 场景，测试数据通过 `fill_data` 工具注入，不把密码值放进 prompt，执行后调用 finish 回报。
- MCP：新增 `POST /mcp` JSON-RPC HTTP 入口，首批工具 `list_cases`、`create_test_task`、`get_run_status`、`get_run_report`、`list_bugs`、`retest`；使用 `X-Agent-Token` 鉴权，不开放关闭缺陷或审批补丁。真实验证 initialize、tools/list、无效令牌 401。
- 验证：Go `go test ./...`、`go build` 通过；Python runner 导入/编译通过；设置接口验证加密存储、普通接口不返回明文、执行端能取配置；MCP 工具发现和鉴权通过。
- 仍未完成：正式用户登录/RBAC、前端设置页、MCP streamable HTTP/SSE 完整协议、接口自动化 Agent、性能测试 Agent、白盒测试、正式报告导出、桌面端 exe/dmg、生产级设备令牌与密钥轮换。

## 并行完善计划（2026-11-22）
- 用户要求启用多智能体并行开发，尽快完善平台。
- 本轮并行范围：补租户模型设置 UI；新增确定性接口自动化执行器；修正 UI 执行通过判定并按测试类型路由。各任务写入文件互不重叠，完成后由主控集成验证。
- 登录与 RBAC 仍属于最高优先级缺口，但首个管理员开户和用户邀请流程尚无已确认方案，本轮先不擅自改变业务身份模型。
- 远程仓库 `https://github.com/DamonZS/Auto-Test` 当前为空仓库（GitHub API 返回 409 Git Repository is empty）；本地工作目录目前没有 `.git`。本轮在现有本地源码上开发，不推送远程。
- 验证基线：`platform/web` 的 `npm run build` 通过；`platform/server` 的 `go test ./...` 可运行但仓库没有 Go 测试；系统 Python 环境没有 pytest。后续使用项目可用的解释器和命令复验。

## UI 设计确认（2026-11-22）
- 品牌统一为 TopoReduce，使用用户提供的公司圆环 Logo；监控页、用例工作台、窗口标题使用相同品牌标识，不再使用 TestPilot、Record Studio 或 Auto-Test 作为界面品牌。
- 当前 UI 落地在 `record-studio/frontend`（Vue 3 + Vite），保留原用例录制工作台；新增默认监控首页，数据沿用现有 FastAPI 接口，不改后端业务。
- 视觉族：B · 管理台；高密度；固定左侧导航；1440px 桌面基线；不用 3D；浅色/深色双主题。
- 首页顺序：最近测试活动为第一阅读点，其次为运行中任务与失败告警；不突出单一 KPI。运行数据每 3 秒刷新；执行详情用右侧抽屉展开。
- 动效预算：中等，支持 `prefers-reduced-motion`。秒级刷新仅更新运行数据，不让整页跳动。

## 本轮并行开发结果（2026-11-22）
- [x] 设置页：`platform/web/src/App.vue` 增加设置入口，新增 `platform/web/src/views/SettingsView.vue`，`platform/web/src/api.js` 增加 PUT 请求。API Key 只显示已配置状态，输入框不回显旧值，保存后清空输入框。
- [x] 确定性接口执行器：新增 `agents/api_runner.py` 与 `tests/test_api_runner.py`。支持 HTTP 请求、状态码、JSON 路径等于、JSON 路径存在、响应耗时断言，限制请求不跨 `base_url` origin，报告不写请求凭据和响应敏感值。无断言返回 `needs_review`。
- [x] UI 执行结果门禁与测试类型路由：更新 `agents/ui_runner.py`，新增 `tests/test_ui_runner.py`。只有显式 `wait_text` 页面断言实际执行且全部通过才返回 `passed`，UI/API/perf/未知类型分别路由，未实现或不可用的执行器返回 `needs_review`。
- [x] 验证：`platform/web` 的 `npm run build` 通过；`spike_deepagents/.venv` 下 `python -m unittest tests.test_api_runner -v` 通过 5 项、`python -m unittest tests.test_ui_runner -v` 通过 9 项，Python 编译检查通过；Go `go test ./...` 运行成功但当前没有 Go 测试。
- [ ] 本轮未完成：性能测试执行器、正式用户登录/RBAC、MCP sampling 与完整 streamable HTTP、多租户管理界面、白盒测试、桌面安装包、生产级 API key 管理与密钥轮换。
- [ ] 代理运行限制：DeepSeek 子智能体因 401 无效令牌失败，GLM Flash 子智能体因模型不可用失败，后续使用可用工作模型完成了本轮文件修改与验证。
- 集成验证补充：项目声明的 pytest 已临时安装到 `spike_deepagents/.venv` 用于验证，未修改依赖文件；本轮新增测试与既有 `tests/test_scenarios.py` 分开执行时通过。全量 pytest 仍被既有 Excel 夹具列数与参数名不匹配阻塞，未修改该范围外的旧测试。

## 测试流程文档参考及后续实现（2026-11-22）
- 用户提供 `docs/体系文档` 与 `docs/测试输出文档` 作为测试流程和报告模板参考。抽取到的通用企业流程为：测试申请与计划 -> 准入检查 -> 需求分析与追溯 -> 用例设计与同行评审 -> 测试环境/数据准备 -> 冒烟与正式执行 -> 缺陷分级跟踪、修复、复测及重开 -> 准出评审 -> 测试总结报告；执行期间需有日报/进度、阻塞问题与缺陷统计。
- 用例字段参考包含唯一编号、业务/需求或模块、测试项、用例描述、预置条件、输入数据、步骤、预期结果、优先级/级别、执行结果和备注；评审应保存评审人、结论、问题与整改跟踪。测试数据应关联具体用例，可区分前置数据与结果数据。缺陷与报告需能统计严重度、优先级、状态、修复轮次、关闭/重开及遗留风险。
- 报告结构需覆盖测试范围与环境、起止时间、有效/执行用例数、需求覆盖率、执行率、通过率、缺陷总量及严重级别分布、遗留缺陷、结论、风险建议与审核签字信息。指标必须由平台执行数据计算，不能用样例模板中的旧数值或公式错误作为产品默认值。
- 原始参考资料包含保险业务样例、企业专有字段和旧版签字/工具流程。平台仅抽象通用字段和阶段，不复制样例客户信息，不要求使用特定企业外部工具。

## 本轮增量实现与验证（2026-11-22）
- [x] Python 性能执行器 `agents/perf_runner.py` 已加入，并由 `agents/ui_runner.py` 将 `type=perf` 路由到执行器。执行器对配置目标发出真实本地/受控 HTTP 请求，统计请求量、失败数和响应耗时指标；专项验证覆盖错误响应、超时和报告敏感数据处理。
- [x] MCP sampling 请求构造和响应解析纯函数新增于 `platform/server/internal/api/mcp_sampling.go`，测试在同目录 `mcp_sampling_test.go`。请求保留租户上下文供服务端调用，协议参数不携带非标准 tenant 字段；模型输出只解析为 `needs_review` 草稿，不持久化原始源码。
- [x] Go 性能任务链路测试新增于 `platform/server/internal/api/perf_test.go`。复用已有通用 case/run/heartbeat/finish 接口验证 `type=perf` 数据在 Go 队列与 Python worker 契约间传递，并覆盖租户隔离。未新增独立性能 API 路由。
- [x] `record-studio/frontend` 增加测试数据概览、测试数据管理、执行/缺陷/报告入口和设置页导航适配，已有录制工作台保留。
- [x] 复验：`platform/server` 下 `go test ./... -count=1` 通过，7 项测试、3 个包；`go build ./...` 通过；项目虚拟环境下 Python `unittest tests.test_ui_runner tests.test_api_runner -v` 通过 14 项，`pytest tests/test_perf_runner.py -q` 通过 4 项；`platform/web` 与 `record-studio/frontend` 两个 Vue 应用的 `npm run build` 均通过。pytest 有一条现存 `asyncio_mode` 配置项警告。
- [ ] MCP sampling 仍未端到端接通。当前 `POST /mcp` 是同步 HTTP JSON-RPC 单请求入口，没有 Streamable HTTP 会话/双向请求生命周期，因此 sampling helper 尚未被 `generate_test_cases_from_code` 调用，也没有创建人工审核草稿。不能对外标记为 MCP 模型生成已完成。后续需要先实现并验证支持客户端响应 server-initiated `sampling/createMessage` 的传输会话，再接数据库草稿写入。
- [ ] 企业流程产物仍不完整：测试申请、准入/准出检查单、计划/评审记录、日报、正式总结报告模板尚需产品化成可编辑实体和导出文件；当前前端页签/概览入口不能代表这些完整工作流已交付。
- [ ] 本轮 `record-studio/frontend` 的“测试数据”入口明确使用当前浏览器 localStorage，供旧录制工作台本地整理，不是 Go 企业平台的租户持久数据源；Go 企业平台的数据集生成与存储页面位于 `platform/web/src/views/TestDataView.vue`，继续以该页面和 Go API 为正式产品链路，避免把 localStorage 页面误认为已接入租户数据服务。
- 验证过程中发现早期并行代理的 Go 测试结果是在 `perf_test.go` 未落盘时产生的失败；文件随后已加入，本轮主控复验以最新完整测试结果为准。

## 需求变更：迁移旧源码并移除旧项目目录（2026-11-22）
- 用户明确要求：将 `record-studio/`、`laya-pilot-main/`、`lemon-TestPilot-main/` 中仍有价值的产品源码及必要文档/测试提取整合进主平台，验收后项目中不再保留这三个目录。
- 主平台保持既定 Go 控制面、Vue 3 主工作台、Python Agent/执行器架构。禁止为迁移保留第二套可部署后端或平行产品前端。
- 迁移采用能力对照而非目录复制：已有等价实现的能力用主平台文件与真实测试证明覆盖，不重复搬一份；仍缺少的独立能力按主平台结构迁移并补真实实现测试。设计参考、协议、算法和重要配置说明迁入 `docs/` 或 `Agents.md`。运行数据库、截图、缓存、构建产物、临时脚本、个人凭据与样例业务数据不属于源码迁移范围。
- 分目录迁移范围：
  - `record-studio/`：对照录制/回放、步骤编辑、用例层级与继承配置、并行会话、计划调度、截图/诊断；仅迁移主平台缺失且确有产品需求的实现，测试通过后删除原目录。
  - `laya-pilot-main/`：对照 Playwright 决策 Provider、Laya 本地/HTTP 候选概率契约、页面 readiness/DOM/Form 语义适配、自然语言 Excel 用例导入导出/回放完整性、浏览器诊断；只保留主平台可用的 Python 决策/自动化能力、相关测试与必要协议文档，不迁移 Node 演示服务器、公开 demo 或模型权重。
  - `lemon-TestPilot-main/`：对照需求/版本影响追溯、用例评审、缺陷生命周期、测试计划与报告/指标；主平台已有的 CRUD、配置页面不重复迁移。只将尚缺的企业流程模型/行为落入 Go API、SQLite、Vue 3 或 docs，并为实际业务行为加测试，不保留 React/Node 原型。
- 删除准入条件：三目录的源码、必要文档、配置契约与测试均完成能力归属登记；迁入实现从主平台正常入口可用；目标模块测试、Go 全量测试与构建、Python Agent 测试、主前端构建通过；全仓引用扫描无运行时代码依赖。满足后在同一最终 `main` 提交中删除三个旧目录、更新忽略规则与历史架构中的当前状态说明，并再次从干净 `main` 验证。
- Git 要求：开发和迁移验证期间不推送不完整状态；所有迁移变更与指定目录删除汇总到 `main`，完整验证后再一次性推送远端。
- [ ] 建立三目录到主平台的逐项能力映射及不迁移清单。
- [ ] 完成并验证 record-studio 独有源码能力迁移。
- [ ] 完成并验证 laya-pilot-main 独有源码能力迁移。
- [ ] 完成并验证 lemon-TestPilot-main 独有源码能力迁移。
- [ ] 删除上述三个目录并完成干净 main 全量验证后推送远端。

## 旧源码迁移审查增量（2026-11-22）
- 三个只读能力审查使用 `p-gpt/gpt-6.2-sol`。完整映射见 `docs/migrations/legacy-capability-map.md`。
- [2026-11-23] 用户指定拓扑模型。后续所有子代理创建时显式使用 `toporeduce/gpt-6.2-sol`，并行处理写入范围独立的任务。
- `record-studio`：主平台已有单用例编辑/回放和单用例周计划，不等于旧平台的套件。套件、套件级计划/并发/汇总报告、录制、节点树/属性继承、API 响应变量及逐步骤证据仍未迁移。
- `laya-pilot-main`：Laya 核心候选/概率 API 已迁移；Excel 导入导出与隐藏步骤一致性、页面 readiness、frame/shadow DOM、行/弹层定位、Form 专项和浏览器网络/页面事件诊断脱敏仍未等价迁移。
- `lemon-TestPilot-main` 同时含可运行 SQLite 工作台、规则型工作流 API、独立模型调用入口和静态设计原型。七类入口、字段完整性、顺序工作流和人工复核约束作为静态领域定义迁移；不得声称这些定义已连接 Deep Agents 或真实模型。测试申请/准入、报告模板与完整度规则仍待完成或明确暂不纳入版本。
- 本轮修复 legacy case history：更新没有初始 revision 行的旧草稿时，事务先补记旧场景版本再写新版本。Go 回归测试验证升级后能查询 v1/v2。
- 旧目录仍不得删除，直到能力逐项完成或明确登记为本版本不支持，并通过目录删除后的全量验证。
- readiness 基础实现现为 `auto_ai/readiness.py`，真实 Playwright readiness 测试 5 项通过。诊断基础实现现为 `auto_ai/browser_diagnostics.py`，标准库测试 5 项通过；尚未接入 UI runner 的报告持久化。Lemon 七类入口定义已迁入 `agents/workbench_catalog.py`，测试 6 项通过；不代表已有模型执行链。
- [2026-11-22] 当前模块验证：Readiness Playwright 5 项、浏览器诊断 5 项、Workbench catalog 6 项、Laya provider/UI/perf runner 19 项通过；Go 全量测试 17 项通过，主 Web `npm run build` 通过。Pytest 发出 `asyncio_default_fixture_loop_scope` 配置警告。
- [2026-11-22] `.gitignore` 新增平台本地 SQLite、WAL/SHM、加密 `.key` 和 `.codegraph/` 忽略规则；运行数据库文件从 Git 索引移除但本地保留。迁移完成前还需复查文件完整性和测试启动行为。

## 主平台迁移交付（2026-11-23）

- 本轮工作区统一采用 Go 控制面、Vue 3 工作台和 Python 执行器。套件成员与顺序保存在 SQLite。手动执行和每周计划在同一事务内登记批次及子执行，复用租户队列。工作台可查看批次历史和各结果状态。
- 结构化 UI 复用 Playwright 导航就绪检查与断言。浏览器错误按类别限量采集，摘要通过 worker 回传写入运行报告。Laya 决策仍是可关闭增强档。
- 录制事件转换、结构化 Excel 往返通过 `scripts/case_cli.py` 提供离线入口。旧 Excel 自然语言联调以 `AUTO_AI_E2E_WORKBOOK` 显式启用。Lemon 七类入口和报告审核规则已进入 Python 领域模块，产品接入待完成。
- Go 同时提供静态 Web 和 `/mcp`。当前已验证同一进程可发现 MCP 工具。MCP Sampling 会话、正式认证和客户端逐项联调仍待完成。
- 此次交付范围为已实现且验证的迁移增量。录制会话、复杂 DOM/Form、套件并发配置和企业测试流程的剩余项逐项列在 `docs/migrations/legacy-capability-map.md`。原始旧项目保存在仓库之外，便于后续提取。
- GitHub 首次发布采用清理后的源码快照。数据库、加密密钥、原始企业样表与报告留在本机。旧本地提交保留供恢复和追溯。
- 本地归档位于 `D:/Project/Autocomplete-Test-legacy-2026-11-23`。实测 Go 测试通过，Python 本地测试 89 项通过、1 项联调按配置跳过，Vue 生产构建通过。真实 Chromium 访问本地 Go 服务，验证套件创建、成员排序、入队、批次汇总及每周计划保存和删除。

## 剩余交付增量（2026-11-23）

- 工作台与部署沿用 Go 控制面、Vue 3、Python 执行器和 Deep Agents。仓库根目录的 `.env` 仅用于本地配置。生产启动使用正式认证。预配 owner 后才可登录，工作区根据成员角色授权。Web 会话、worker 令牌、MCP 令牌分开签发和校验。旧库业务数据保留，迁移和轮换流程见 `docs/api/auth-migration.md`。
- Web 录入需求、环境和用例，按同租户项目建立关联。任务发布后，worker 从 Go 领取场景、环境地址和绑定的测试数据行。Worker 每次领取后读取最新模型设置。API、性能和结构化 UI 回放可在未配置模型时执行。自然语言 UI 缺模型配置时进入待复核。
- 套件批次记录成员顺序和并发上限快照。手动与计划批次共用入队逻辑。SQLite 原子领取控制批次正在运行的任务数，超过上限的任务继续排队。默认上限为 1，配置值高于 1 时需要运行多个 worker。
- Playwright 结构化回放新增单层 iframe、行上下文、开放式 Shadow DOM 和原生复选或下拉控件。逐步状态及耗时与运行结果同事务保存，详情按租户返回。录制端通过 `case_cli.py record-ui` 捕获主页面导航及具名控件操作，仅保存填写字段引用。Web 可导入生成的 JSON，在用例页补充断言和测试数据并评审。
- 执行机以 `PLATFORM_BROWSER` 选择 Playwright 的 Chromium、Firefox、WebKit，或已安装的 Chrome、Edge 通道。缺少所选浏览器时报告启动错误。
- [2026-11-23] 设置页可按租户选择 Playwright 浏览器，正式 worker 领任务后读取最新选项。选择「执行机默认」时继续使用 `PLATFORM_BROWSER`，未配置环境变量时使用 Chromium。已实测 Firefox 启动和设置页保存至真实回放。Ego v2.2.1 可运行本机合成表单冒烟，租户任务继续使用 Playwright。JEV 缺少已验证的服务接口，请求 Ego 或 JEV 时在 UI 运行报告记录降级。配置见 `docs/deployment/browser-providers.md`。
- 支持 Sampling 的 MCP 客户端通过 Streamable HTTP 接收原请求流上的模型请求，用另一个 POST 回包。源码只在请求过程中使用。首版从校验后的模型建议生成项目内 GET/HEAD 成功状态断言或 UI 首页文本断言。初始修订、草稿和审计同事务保存。草稿定稿后才可执行，报告返回实际执行结果和复测提示。客户端 Sampling 能力及反代配置见 `docs/api/mcp-sampling.md`。
- 企业流程增加测试申请、计划、准入、同行评审、日报、准出、总结报告七类可编辑产物。保存使用修订号。提审校验必填内容，日报和正式报告冻结提交时的版本统计。认证用户签核后可下载 HTML。
- 版本页支持导入 Go coverprofile 与 Cobertura 白盒覆盖率，仅保存汇总计数。日报和正式报告在提审时一并冻结覆盖率。完整白盒执行范围仍需确定。
- 可选 Chroma 使用每租户 collection 和租户、项目元数据过滤。启用的 worker 领取 UI 任务时可取得已录入的项目需求，为自然语言 UI 执行检索相关片段。SQLite 保存原始需求，索引可重建。
- Linux 服务端以 Compose 发布。桌面目标为 Windows x64 安装程序和 macOS arm64 DMG，包含 Go 服务、Vue 工作台、Python worker 与 Chromium。程序和用户数据分离。平台构建及首次安装流程见 `docs/deployment/README.md`。
- 干净环境发布验收发现历史 LangGraph 入口在包导入时即初始化模型。现按需加载旧入口，确定性 worker 启动可不提供模型密钥。回归测试在独立进程关闭 dotenv 并清除模型凭据。

### 发布验收

- 发布源提交 `201e08f093ce778c4b6cc3027e383e5e158b5034` 的 GitHub Actions 运行 `45079266280` 三项通过。Linux 验证 Go/Web 与 worker 的 Compose 启动、真实 Chromium 执行及重启后的结果持久化。Windows x64 验证安装、正常领取和完成任务、停机及卸载。macOS arm64 验证 DMG 构建、挂载及从安装副本执行相同任务。生产托管站点尚未配置。
- 本地验证包括 Go 56 项、Python 无模型密钥回归 102 项、Chroma 专项 5 项、Vue 生产构建、Microsoft Edge 复杂表单真实回放。Python 回归中的 5 项跳过包括 4 项已在独立 Chroma 环境通过的用例，以及 1 项需显式配置外部站点工作簿的用例。
- 交付产物为 Windows x64 EXE、macOS arm64 DMG 和 Linux amd64 容器归档。桌面交付当前适用内部验收，Windows 签名及 macOS 公证待正式发布配置。首次启动及旧库迁移流程见 `docs/deployment/README.md`。
- 待接入项包括实际 coding agent 客户端的 Sampling 联调、ego 和 JEV 增强档、工作台直接启动录制、逐步截图、旧工作簿兼容、企业产物的模型生成和更完整的白盒执行。各项以独立实测证据更新状态。

## 桌面窗口与 GitHub 发布（2026-11-23）

- 用户确认本阶段暂缓 Windows 安装包签名及 macOS 应用签名和公证。现阶段按现有构建、验收及 GitHub Releases 流程交付桌面安装包。
- Windows 与 macOS 安装版通过独立应用窗口承载工作台，随包提供 Qt WebEngine。应用窗口、开始菜单、托盘和 macOS 应用包使用 Auto-Test 图标。关闭窗口收起到托盘，从托盘菜单退出会停止本机控制面与 worker。Windows 停止快捷方式显示实际停止结果。
- 桌面首次打开时在窗口内创建工作区管理员。桌面控制面以本机回环地址启动并开启注册，首个管理员创建后回到常规登录流程。容器部署通过既有命令预配账号。
- 推送 `vX.Y.Z` 标签时运行 Linux、Windows 和 macOS 构建与验收。发布任务在三平台通过后收集三个安装包和校验文件，再公开 GitHub Release。对应中文说明置于 `docs/releases/`。
- `v0.1.0` Windows 安装包使用独立临时数据库实测首次注册、自动登录和任务回报。原生窗口实测关闭后收起、再次启动恢复、停止服务与卸载。发布构建运行见 GitHub Actions `45094204345`。
- 用户指定橙色外环与青色内环的公司 Logo 作为桌面品牌图标。PNG、ICO 和 ICNS 从同一原图生成，保留透明通道。
- Windows 安装程序与卸载界面使用简体中文。覆盖安装检测到旧版运行时，允许选择关闭旧版或取消安装。确认后通过应用自己的停止入口退出，停止失败则在覆盖文件前报错。关闭会中断正在执行的测试，用户数据目录保留。

## 需求、录制和回归执行（2026-11-23）

- 需求变更来自用户对桌面工作台的追问。工作台提供脚本录制回放、定稿用例执行、按需求自动生成测试草稿、需求文件导入和需求修订追踪。
- 「需求」支持 UTF-8 CSV 批量导入，表头为 `code,title,content`。Markdown、TXT 每份文档导入一条需求。编辑追加快照。旧库启动时为现有需求补首版快照。项目版本关联某个修订后可比较前后文本，并统计直接关联的用例。API 见 `docs/api/requirements.md`。
- 「需求 → 生成用例」和「用例 → 按需求生成用例」调用租户设置里的 OpenAI 兼容模型。默认使用当前需求修订，也可选用已关联的项目版本。草稿记录生成所依据的修订号与项目版本。首版支持 UI 首页文本断言、接口 GET 和 HEAD 状态码断言。生成、审核与执行分开。契约见 `docs/api/case-generation.md`。
- 桌面工具栏「录制用例」在独立 Chromium 窗口记录主页面导航、具名控件点击和输入字段引用。停止后导出 JSON，用例页「导入录制脚本」支持复核、补断言、绑定测试数据和提交审核。录制范围见 `docs/migrations/recording-cli.md`。
- 定稿的结构化 UI 用例使用「回放」，其他定稿用例使用「执行」。提交后进入对应执行记录并刷新状态，Go 排队、Python worker 执行，结果和逐步证据流回工作台。生成草稿在 Web、MCP、套件和计划执行路径均需先定稿。
- 桌面 XLSX 用例复用现有 CLI 的结构化工作簿读写，通过 Qt 文件对话框在本机转换。旧版 `url`、`scenario`、`name` 自然语言表可导入第一张工作表并逐条选择。用例页事件与打包清单约定见 `docs/migrations/case-workbooks.md`。
- 结构化 UI 步骤支持显式 `screenshot: true`，成功或失败后均可采集遮罩后的视口 JPEG。按运行限制截图次数和单图体积，沿用租户隔离的逐步证据存储与运行详情。CasesView 的新建及草稿编辑步骤控件通过布尔字段接入，契约见 `docs/api/run-step-evidence.md`。
- 临时工作区实测 CSV 导入、需求修订、版本关联、UI 与接口草稿生成、审核定稿、真实 Playwright 与 HTTP 回归，以及 JSON 导入和逐步证据。生成验证使用本地 OpenAI 兼容测试端点。Go 全量 81 项通过，Python 全量 104 项通过、5 项跳过，另有 17 个子测试通过，Vue 生产构建通过。

## 平台功能接入与文档导入（2026-11-23）

- 用户要求现有功能从平台操作，需求页支持上传说明书。上传支持 PDF 文字层、Word DOCX 段落与表格、Excel XLSX 需求行或可见表格数据、UTF-8 CSV、Markdown 和 TXT。先预览，再在当前项目事务写入需求与首版修订。原文件只在请求期间处理。纯扫描件可先 OCR，旧 `.doc` 和 `.xls` 可先另存为现代格式。契约见 `docs/api/requirements.md`。
- 桌面执行端在「设置」激活后领取当前工作区的任务，服务器版继续使用作用域受限的 worker 令牌。MCP 接入页提供服务地址、令牌签发与撤销。客户端采样能力由协商结果决定。
- 用例页接入接口断言与性能参数表单、用例修订历史、桌面 XLSX 导入导出和逐步截图选项。执行记录展示性能指标、逐步耗时和截图，并可下载单次 HTML 报告。缺陷页支持人工提交、调整级别和发起复测。企业流程的七类产物可从租户模型生成草稿并人工签核，概览支持环比同比。
- Windows 运行窗口的原生图标已核验为橙青双环。为避开任务栏旧资源缓存，升级安装生成指向 `autotest-company.ico` 的开始菜单快捷方式。运行中应用和原固定项可在下次打开时刷新，若固定项仍显示旧缓存，从更新后的开始菜单入口重新固定。

## 概览图表与 AI 测试数据（2026-11-23）

- 概览保留浅色侧栏管理台。导航与指标卡使用随应用分发的线性图标。工作区的每日执行结果显示为堆叠柱，通过率与新增缺陷显示为折线，严重程度和当前缺陷状态分别显示为环形图和横向条形图。每日趋势可选近 7、14 和 30 天，UTC 自然日按执行结束、缺陷创建时间聚合。缺陷分布为工作区当前快照。没有明确通过或失败结果的日期在通过率折线中留空。
- 测试数据支持选择项目和可选环境，组合多类字段预设，也可单独勾选字段或补充正常、边界、异常场景说明。复用租户配置的 OpenAI 兼容模型生成合成数据，校验后在同一事务保存数据集和记录，页面自动打开生成结果。密码和证件号用合成值，敏感列在记录预览中遮码。手动按规则生成仍可单独使用。

## 项目化 AI 测试工作台（2026-11-23）

- 用户要求按照既有体系文档、测试输出模板和高级交互动效手册重做全平台流程。项目是工作入口，项目上下文贯穿需求、版本、环境、用例、套件、执行、缺陷和报告。企业流程产物在项目质量交付链路内呈现。
- 需求解析为独立可验收功能点，人工评审可退回并提交意见，AI 基于意见生成修订，批准后作为测试设计来源。功能、接口、UI 自动化、性能与防御性检查按各自契约生成草稿并由人工审核。
- 采用动效手册的按压反馈、列表详情、页面过渡和加载状态，遵守系统减少动态效果设置。已核对禅道缺陷处理、GitLab 修订比较和部署关系，以及 Azure Test Plans 的套件组织。流程、交互和来源见 `docs/workbench-redesign.md`。
- 项目页支持搜索、查看和编辑项目，进度按已批准用例数与总用例数计算，指标可跳转到项目内页面。环境级别和部署版本分别登记。版本页比较两版登记的提交引用与需求修订关联，保留白盒覆盖率导入。接口与迁移见 `docs/api/project-workspace.md`。
- 用户要求切换工作区和退出登录固定在侧栏左下角。主区与侧栏菜单滚动时，两个操作保持可见。

## 桌面交互与 UI 自愈（2026-11-23）

- 全站工作台下拉菜单平滑展开，鼠标悬停使用中性浅灰，仅当前选项带有选中勾标。键盘操作与减少动态效果设置继续生效。版本和环境工具栏筛选项按实际用途使用紧凑宽度。
- 手工需求录入分为编号和标题、正文、底部操作三组，窄窗口使用单列。录制入口移至用例页，命名为「录制 UI 自动化用例」，复用桌面 Chromium 录制器。
- 执行详情展示 UI 定位恢复过程与断言结果。可验证的定位变化形成待审批补丁。审批前保留原用例，批准时创建用例修订并记录来源执行。该流程见 `docs/api/ui-healing.md`。
- GitHub 首页以英文 `README.md` 为主，提供 `README.zh-CN.md`。仓库 About 用英文介绍项目，项目源码和文档采用 MIT，根目录保留许可证和中英版权声明，第三方材料保留原许可证及归因。
