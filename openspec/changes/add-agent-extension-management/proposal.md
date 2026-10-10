## Why

智能体目前仅有编译期 Skill 和固定工具。平台管理员需要统一管理 Skills 与 MCP，并让文字、实时 Agent 实际使用启用配置。

## What Changes

- AI Provider 入口提供“提供商 / Skills / MCP”三个 tab，保留供应商已有配置。
- 平台级自定义 Skill 的新增、编辑、启停、删除与正文管理，展示原有只读内置巡检 Skill。
- 平台级远程 Streamable HTTP MCP 连接管理、加密 Bearer 凭据、连接测试及工具发现。管理员逐个选择禁用、只读或需确认。
- Agent 按需发现及加载 Skills，发现并调用允许的 MCP 工具；需确认工具使用现有会话授权卡片，授权后真实调用并回执。

## Capabilities

### New Capabilities

- `agent-extension-management`: 平台配置、Agent 运行时加载和 MCP 授权调用。

### Modified Capabilities

无。

## Impact

新增平台注册表迁移、Go 官方 MCP SDK、管理 API、前端 tabs 与文字/实时工具编排。无项目级配置，配置写入仅平台管理员；调用仍核验当前成员、会话及 Agent 权限。凭据加密，不返回、不进入模型上下文。原有内置 Skill 与平台工具保留。非目标为执行本地命令、上传执行脚本、旧 SSE/stdio 或 OAuth 接入。
