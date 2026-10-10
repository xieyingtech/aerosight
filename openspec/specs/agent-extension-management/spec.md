# agent-extension-management Specification

## Purpose

让平台管理员在统一 AI 管理入口配置可复用 Skills 与远程 MCP 服务，使项目智能体能按需加载启用的技能和实际调用允许的外部工具，同时保留平台权限、会话授权、审计与凭据保护。

## Requirements

### Requirement: Platform administration tabs
管理页 SHALL 提供“提供商 / Skills / MCP”三个 tab，保留供应商行为。自定义 Skill 和 MCP 配置 SHALL 为平台级，仅平台管理员能新增、编辑、启停、删除；写操作 SHALL 审计，凭据 SHALL 加密且不回显。

#### Scenario: Platform admin edits extensions
- **GIVEN** 登录用户为平台管理员
- **WHEN** 在任一扩展 tab 保存配置
- **THEN** 配置持久化且所有项目 Agent 的后续调用看到最新启用状态

#### Scenario: Non-admin attempts configuration
- **GIVEN** 普通用户或仅项目管理员
- **WHEN** 访问扩展管理 API
- **THEN** 拒绝管理且不泄露配置凭据

### Requirement: On-demand skill loading
文字与实时 Agent SHALL 能发现和加载启用的自定义 Skill，返回名称、版本和正文；内置巡检 Skill SHALL 继续可用且只读展示。停用或删除 Skill SHALL 不再可加载。Skill SHALL 不授予额外权限或运行脚本。

#### Scenario: Enabled custom skill
- **GIVEN** 管理员保存启用的 Markdown Skill
- **WHEN** Agent 发现目录并按名称加载
- **THEN** 正文进入真实模型工具上下文，继续应用平台权限和操作确认规则

#### Scenario: Disabled skill
- **GIVEN** 自定义 Skill 已停用或删除
- **WHEN** 模型继续请求加载
- **THEN** 明确不可用，不能返回旧正文或伪称已加载

### Requirement: MCP discovery and controlled execution
系统 SHALL 支持远程 Streamable HTTP MCP 初始化、工具发现与真实调用；工具 SHALL 默认禁用，管理员逐个设置只读或需确认。Agent SHALL 仅看到启用连接且允许的工具，每次调用验证最新配置、真实 schema、输入和当前会话权限；模型 SHALL 不选择项目或用户范围。

#### Scenario: Read-only call
- **GIVEN** 启用连接的工具已被管理员标记只读
- **WHEN** 授权 Agent 提供合法参数调用
- **THEN** 对真实 MCP 服务执行并将有界结果提供给模型，失败不得宣称成功

#### Scenario: Schema drift or disabled endpoint
- **GIVEN** 工具先前已发现
- **WHEN** 连接停用、工具禁用或远端 schema 改变
- **THEN** 拒绝旧调用并提示重新发现或配置，不自动放行新工具

### Requirement: Confirmed MCP actions
需确认 MCP 调用 SHALL 创建当前用户和会话的待确认卡片，点击前不调用外部工具。批准 SHALL 重新核验配置版本与权限，一次执行并保存结果；重复批准 SHALL 不重复调用。配置变更 SHALL 使旧审批失效，网络不确定 SHALL 提示核对且不自动重试。

#### Scenario: User approves action
- **GIVEN** Agent 已创建待确认 MCP 调用
- **WHEN** 当前用户点击批准
- **THEN** 仅此调用真实执行一次，返回成功、失败或待核对结果并进入会话

#### Scenario: Approval invalidated
- **GIVEN** 待确认调用绑定旧连接 revision
- **WHEN** 管理员修改配置或用户权限被撤销后批准
- **THEN** 不执行旧调用

### Requirement: Credential and network boundaries
连接凭据 SHALL 不出现在公共 API、审计、工具结果或模型上下文；URL SHALL 不内嵌凭据或 query token，外部 HTTP SHALL 固定已解析目标且拒绝重定向。扩展 SHALL 不绕过内部项目访问控制。

#### Scenario: Endpoint changes or output contains credential
- **GIVEN** 已保存 Bearer 的连接
- **WHEN** 修改端点或远端结果回显 Bearer
- **THEN** 不把旧凭据静默发送到新端点，结果中清除凭据
