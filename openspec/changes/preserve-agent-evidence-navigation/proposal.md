## Why

智能体回复中的项目链接使用普通锚点，点击核验设备或证据时触发整页加载，现有悬浮会话随之丢失。以智能体串联巡检流程时，需要在查看业务页面的同时保留同一会话。

## What Changes

- 项目内页面链接使用客户端导航，复用现有 AgentWorkspace 的悬浮行为。
- 外站、协议相对链接与 API/下载地址保留普通链接行为。
- 不改变智能体工具权限、飞行审批、设备控制或任务状态。

## Capabilities

### New Capabilities
- `agent-evidence-navigation`: 项目证据链接跳转时保持智能体会话。

### Modified Capabilities
无。

## Impact

仅影响 ChatMarkdown 的内部链接渲染，复用 Next.js Link，无新依赖或数据迁移。项目数据读取仍由原业务 API 校验成员权限；外部链接不进入客户端路由。不调整录像里的业务事实和历史飞行回执。
