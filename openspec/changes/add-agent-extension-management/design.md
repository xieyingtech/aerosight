## Context

当前 Go 服务有固定 load_skill、文字 Responses 编排、实时函数工具和 agent_write_approvals。管理操作已有审计及平台管理员校验，外部供应商 HTTP 有 DNS pinning 和拒绝重定向机制。

## Goals / Non-Goals

**Goals:** 在同一入口完成配置与实际调用，保留内置技能与项目权限，复用现有确认 UI。

**Non-Goals:** 本地命令运行、Skill 脚本执行、旧 SSE/stdio、OAuth 和项目级注册表。

## Decisions

- Skills 保存 slug、名称、简介、Markdown 正文、启用状态及 revision；内置巡检技能通过既有嵌入文件只读展示。不导入执行附件。通过 list_skills / load_skill 渐进加载，不将所有正文塞入每轮系统指令。
- MCP 保存名称、Streamable HTTP endpoint、加密 Bearer、启用状态、发现工具快照与每工具策略。禁用为默认；只读须管理员明确选定，不凭远端 annotations 自动放行。无项目数据共享缓存。
- 官方 Go SDK v1.2.0 建立短期会话，初始化、分页 tools/list、tools/call 后关闭。复用管理员可信 LAN 地址策略、DNS pinning 和拒绝跨端点重定向；禁止 URL 内嵌凭据和 query token。端点变更需重新提供或明确清除凭据，并清空发现快照。
- list_mcp_tools 返回启用且允许的工具 schema，call_mcp_tool 使用 serverId/toolName/arguments。每次重新校验配置、实际 schema 和输入；模型无法提供项目/用户范围字段，服务附带真实项目和用户上下文 header。连接级管理员凭据仅供已配置外部服务，不授予平台内部项目权限。
- 只读调用直接执行；需确认调用保存配置 revision 与参数到既有审批。点击后锁定审批并重新验证配置/工具 schema/会话权限，一次进入 executing 后不自动重试；不确定网络结果保持待核对状态。批准结果作为普通工具内容，不能提升远端输出为系统指令。
- 外部结果只取有界文本及结构化 JSON，清除 Bearer 内容，无远端图像自动下载、无工具引用自动赋予平台证据可信度。远端失败与 schema 漂移明确失败。
- 三个 tab 保留现有 Provider 内容；Skills 编辑 Markdown，MCP 编辑连接、测试发现并逐个选择工具策略。

## Risks / Trade-offs

- [恶意 Skill / MCP 提示] → 仅平台管理员维护；扩展不能修改实际权限或绕过点击确认；远端描述和输出仍为工具材料。
- [审批后配置变化] → revision 绑定，重新发现漂移拒绝执行。
- [外部写操作超时] → executing 状态不自动重试，提示核对外部系统。
- [共享凭据指向共享外部数据] → 管理页明确说明连接平台共享，远端工具须由管理员选择适合全部 Agent 用户的访问范围。

## Migration Plan

新增两张平台注册表，sqlc/schema 同步；审批表现有 tool_name 为开放字段，已有 executing 状态，直接复用，无需扩展约束。在隔离数据库执行迁移与本地 MCP 真实协议验收后再交付。回滚禁用扩展即可保留原有平台工具。
