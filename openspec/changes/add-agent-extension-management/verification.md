# 验证记录

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
