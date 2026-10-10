# 验证记录

## 2026-10-10 红花山公园演示配置

- 用户明确授权在 xieying.tech 创建演示 Skills。使用现有管理员 API 创建五个启用配置（ID 1–5、revision 1）：巡检准备、游客通道、烟火夜间、环境设施、事件证据交接。原有内置 Skill 保留，目录从 1 项变为 6 项；逐项回读正文、名称、简介和启用状态一致。可审查正文与公开来源位于 docs/agent-skills/。
- 真实线上项目 1、会话 5：五次独立加载均 HTTP 201，持久化工具记录 load_skill 均 succeeded，真实模型输出说明版本 1 及对应流程。不是模型协议替身验收。
- 同会话最小巡检准备演示 HTTP 201：query_devices、query_map_context 均 succeeded，返回当前项目两台设备及态势引用。模型如实指出当前设备质量 unusable，未启动飞行、写任务/案件或发通知。
- 浏览器管理员 Skills tab 显示五项启用配置。完整列表截图与 API/Agent 原始验收结果保存在忽略的 .build/online-park-skills* 文件中，不提交凭据或原始项目数据。
- 首次“加载并多步检查”请求遇 HTTP 524，未产生助手结果；拆为单项加载后五项全部成功。最小业务查询第一次遇网络连接超时，随后只读重试成功。多步长链路的线上时延仍需录屏前检查，不把这些重试包装成全链路稳定性或并发性能验收。
- 本次仅配置内容与文档，不变更 API、数据模型、模型部署或权限。对应主规范既有 On-demand skill loading，不产生新的 delta 要求；严格 change 验证与 git diff --check 通过。未重跑代码构建/全量测试，既有失败仍保留，不归档 change。

## 已验证

- 隔离本地 PostgreSQL 迁移执行成功，新增 agent_skills / agent_mcp_servers；sqlc 生成模型和 schema 检查一致。
- `TestAgentExtensionsRealMCP`：在隔离数据库及 SDK Streamable HTTP 真服务验证管理员 CRUD、凭据加密/脱敏、默认禁用、只读实际调用、确认前不执行、重复批准、旧配置失效、成员撤销、schema 漂移、端点变更及删除。
- 该测试通过真实 Responses SDK 请求/工具输出编排（模型响应为协议测试替身），验证自定义 Skill Markdown 和 MCP 结果进入下一轮模型输入；通过 WebSocket 实时模型协议替身验证 Skills 加载与真实 MCP 调用。未声称已验收第三方生产 MCP 或实机语音。
- 不确定外部执行结果保留 executing，重复点击拒绝，工具不会自动重试。
- `TestAgentExtensionEndpointAndSchemaBoundaries`：拒绝 URL 凭据、查询参数、片段及外部 schema 引用。
- 相关 Chat / Realtime / AgentWorkflow / AIProvider / ObjectQuery 回归测试在隔离数据库通过。
- 浏览器完成三个 tab 切换、自定义 Skill 新建保存、本地 MCP 新建、发现两个工具、编辑只读/需确认策略并保存。
- `pnpm typecheck` 与 `pnpm build` 通过；主规范及 change 严格验证通过。

## 既有失败与边界

- 全量 `pnpm check` 未通过：`TestReportAggregateIncludesAllEvidence` 仍断言旧的报告资源链接，与当前规范路由不一致；该失败在本 change 前已存在，报告实现与该测试未修改。全量检查其他执行项通过，依赖数据库环境而未配置的集成测试仍按原规则跳过；本次相关集成测试另行使用隔离数据库执行。
- 本地 MCP 验收服务与样例仅位于忽略的 `.build` 和隔离数据库；未提交临时脚本、配置或凭据，未修改生产数据库或部署生产。
- 支持 Streamable HTTP，未实现 stdio、旧 SSE、OAuth 或 Skill 脚本执行，这些属于明确非目标。
