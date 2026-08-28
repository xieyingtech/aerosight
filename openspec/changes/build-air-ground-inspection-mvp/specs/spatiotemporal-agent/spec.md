## Purpose

定义项目级 AI 聊天与案件 Copilot 委派能力，使回答、案件解释和任务协助均可追溯到项目证据，同时确保模型只能通过 Tasks 与领域安全入口产生副作用。

## ADDED Requirements

### Requirement: 管理后台配置 AI Provider
系统 SHALL 允许平台管理员在管理后台配置平台级 AI Provider 的类型、名称、可选基础地址、默认模型、API Key、启用状态和默认状态；Agent Provider Registry MUST 从数据库加载唯一的启用默认 Provider，且 MUST NOT 从 AI Provider、模型或 API Key 环境变量加载运行配置。

#### Scenario: 管理员创建默认 Provider
- **GIVEN** 平台管理员填写有效 Provider、模型和 API Key
- **WHEN** 管理员测试并保存为启用的默认 Provider
- **THEN** 系统将配置和 AES 加密后的 API Key 保存到数据库，后续智能体聊天、案件 Copilot 和 `copilot.run` 使用该 Provider

#### Scenario: 查看和更新已有 Provider
- **GIVEN** 数据库中已有包含 API Key 的 Provider
- **WHEN** 平台管理员打开编辑页
- **THEN** 系统不在读取响应中返回 API Key，密钥 input 显示为空；保存时留空 SHALL 保持原 API Key，填写非空值 SHALL 覆盖原值

#### Scenario: 非管理员管理 Provider
- **GIVEN** 用户不是平台管理员，即使其是项目 owner 或 admin
- **WHEN** 用户请求读取管理列表、测试、创建、修改、启停或删除 AI Provider
- **THEN** 系统 SHALL 拒绝操作且不得返回 API Key 或可用于推断其内容的数据

#### Scenario: 没有可用的默认 Provider
- **GIVEN** 数据库中没有同时启用且被设为默认的 AI Provider
- **WHEN** 用户使用智能体聊天、案件 Copilot 或任务 `copilot.run`
- **THEN** 系统 SHALL 明确返回 AI Provider 未配置或不可用，且设备、Tasks、案件和其他非 AI 功能继续可用

### Requirement: 基于项目证据的智能体聊天
智能体页面 SHALL 提供绑定当前项目和用户的 AI 聊天；智能体 SHALL 仅查询当前授权项目范围内的设备、任务、观测、案件、资产和证据，并在事实性回答中返回可导航的来源引用、查询时间窗和数据新鲜度。

#### Scenario: 查询当前异常
- **GIVEN** 用户有项目访问权限并询问当前异常
- **WHEN** 智能体调用项目查询工具
- **THEN** 智能体基于返回案件和任务状态回答并引用案件、时间和证据，不得混入其他项目数据

#### Scenario: 没有足够证据
- **GIVEN** 查询时间段无数据或关键来源不可用
- **WHEN** 智能体生成回答
- **THEN** 智能体 SHALL 明确说明未知和缺失来源，不得把推测表达为事实

### Requirement: 授权范围内的平台调度
智能体 SHALL 能将自然语言目标转换为结构化 Task、报告或后续处置草案，并可代表当前用户请求创建或启动其有权使用的 Task；任何请求 MUST 进入 Tasks 的权限、版本校验、预检、审批、幂等和命令账本流程，智能体不得直接向设备、连接器或算法 Adapter 下发调用。

#### Scenario: 创建巡检草案
- **GIVEN** 授权用户描述巡检目标和区域
- **WHEN** 智能体完成任务解析
- **THEN** 系统展示可编辑 Task 草案、缺失参数和假设，并等待用户确认与发布

#### Scenario: 用户要求绕过审批立即起飞
- **GIVEN** 任务包含受保护设备动作
- **WHEN** 用户要求智能体直接执行或忽略安全限制
- **THEN** 智能体和工具层 SHALL 拒绝绕过，并引导用户完成预检与审批

#### Scenario: 有权用户请求调度巡查
- **GIVEN** 当前用户具备任务操作权限并要求无人机重新巡查案件区域
- **WHEN** 智能体构造并提交调度请求
- **THEN** 系统创建绑定用户和案件的 Task 草案或运行请求，并返回预检与审批状态而非声称设备已经执行

### Requirement: 受策略约束的工具调用
每个智能体工具 SHALL 声明输入模式、只读/草案/受保护风险级别、所需项目权限和是否需要确认；工具执行上下文中的用户、团队、项目和会话标识 MUST 由服务端生成而不是来自模型参数，服务端 MUST 在每次调用和后台实际执行时重新校验 scope 与资源边界。

#### Scenario: 调用只读查询工具
- **GIVEN** 用户有项目访问权限且参数通过模式验证
- **WHEN** 智能体调用时空查询工具
- **THEN** 工具返回项目范围内的有限结果并记录调用摘要

#### Scenario: 伪造跨项目工具参数
- **GIVEN** 会话属于项目 A 但工具参数指向项目 B
- **WHEN** 工具服务执行授权
- **THEN** 系统 SHALL 拒绝调用，即使语言模型已生成该参数

#### Scenario: 入队后权限被撤销
- **GIVEN** 智能体已创建后台分析或调度请求，但用户在实际执行前失去项目权限
- **WHEN** worker 重新校验执行上下文
- **THEN** 系统 SHALL 拒绝执行并记录权限变化，不使用入队时权限快照绕过当前 scope

### Requirement: 案件 Copilot 提及与指派
系统 SHALL 允许具有 `agent:use` 权限的用户在案件评论中提及 `@copilot`，并允许具有案件指派权限的用户把案件指派给 Copilot；每次调用 MUST 绑定案件、触发活动、发起用户和当前项目，Copilot 的接收、进度、结果或失败 SHALL 写回案件活动时间线。

#### Scenario: 评论触发案件分析
- **GIVEN** 用户在有权访问的案件中评论并提及 `@copilot`
- **WHEN** 系统验证提及、权限和项目范围
- **THEN** 系统保存用户评论、幂等创建一次案件 Agent Job，并把带稳定证据引用的回复写为 Copilot 评论

#### Scenario: 指派 Copilot 自动处理案件
- **GIVEN** 用户有案件指派和智能体使用权限
- **WHEN** 用户把案件负责人设置为 Copilot
- **THEN** 系统创建绑定该指派活动的处理 Job，并在同一时间线持续展示状态，不依赖项目级自动 AI 设置

#### Scenario: 评论文本不构成有效提及
- **GIVEN** 评论只是引用历史文本、代码块或无权限用户输入 `@copilot`
- **WHEN** 系统解析评论
- **THEN** 系统 SHALL 保存普通评论但不创建 Agent Job，并不得扩大用户权限

### Requirement: 可解释的案件分析
智能体 SHALL 区分已观察事实、Task 条件结论、模型推断和建议，展示关键置信度与反证，并不得自行改变案件的人工结论。

#### Scenario: 解释案件原因
- **GIVEN** 案件存在任务条件、轨迹和检测证据
- **WHEN** 用户询问为何创建该案件
- **THEN** 智能体按证据链解释触发条件、置信度和不确定性，并链接到原始记录

#### Scenario: 证据互相冲突
- **GIVEN** 疑似违建算法、历史影像或人工标注之间存在冲突
- **WHEN** 智能体分析案件
- **THEN** 智能体 SHALL 呈现冲突并建议复核，不得选择一个来源冒充确定结论

### Requirement: 报告生成与版本控制
智能体 SHALL 能基于确定性报告数据生成叙述性摘要和行动建议；生成内容 MUST 记录模型、提示模板、输入证据版本和生成时间，并由用户确认后才能成为已发布报告。

#### Scenario: 生成报告摘要
- **GIVEN** 任务报告数据和证据已准备完成
- **WHEN** 用户请求生成摘要
- **THEN** 智能体生成带引用的草稿并标记建议与事实

#### Scenario: 输入证据发生修订
- **GIVEN** 报告草稿生成后人工结论或证据版本改变
- **WHEN** 用户尝试发布旧草稿
- **THEN** 系统 SHALL 提示草稿过期并要求重新生成或明确确认差异

### Requirement: 会话隔离与最小留存
智能体聊天会话和案件 Agent Job SHALL 绑定项目、发起用户以及可选案件/任务运行，工具结果 SHALL 受项目授权过滤；敏感凭据、签名媒体地址和完整原始流不得进入长期会话或案件评论内容。

#### Scenario: 用户失去项目访问权
- **GIVEN** 已有会话的用户被移出团队
- **WHEN** 用户继续会话或调用工具
- **THEN** 系统 SHALL 拒绝访问项目内容，不以旧会话缓存绕过授权

#### Scenario: 工具返回临时媒体地址
- **GIVEN** 工具为当前请求生成短时签名地址
- **WHEN** 会话消息被保存
- **THEN** 系统 SHALL 保存稳定资产引用而不是临时凭据

### Requirement: Tasks 之外不存在自动执行引擎
Copilot 对平台资源产生的任何自动副作用 SHALL 能归因到已发布 Task 的 `copilot.run` 步骤，临时协助 SHALL 能归因到智能体聊天或案件提及/指派；系统 MUST NOT 通过项目级自动 AI 开关、算法完成回调或检测保存建立另一套后台编排。

#### Scenario: 已发布任务自动调用 Copilot
- **GIVEN** Task 版本明确包含 Copilot 步骤
- **WHEN** 运行到达该步骤且权限仍有效
- **THEN** 系统保存任务版本、步骤、触发主体、模型、工具与输出，后续动作继续由 Task 调度

#### Scenario: 算法运行独立完成
- **GIVEN** 算法服务返回成功结果但当前 Task 没有 Copilot 步骤
- **WHEN** 系统保存算法结果与检测
- **THEN** 系统 SHALL 不自动调用 Copilot；只有后续任务步骤或用户显式交互可以发起 AI
