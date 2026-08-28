## Context

参见 [proposal.md](./proposal.md) 的动机与范围。当前系统由 Next.js Web、最小 Go worker 和 PostgreSQL 组成；同步业务和授权集中在 Web，worker 尚无队列或后台处理实现。现有 `devices`、`device_capabilities`、`tasks`、`task_runs`、`assets`、`issues` 和智能体表可继续作为业务入口，但缺少高频时空数据、设备命令、媒体对象、感知结果和审计结构。

首期设计面向单个部署中的多个 Team/Project，预期每项目为个位至数十个在线设备、秒级遥测和有限路直播。架构必须允许未来增加设备数量和算法处理节点，但首期不以城市级海量低空目标为容量目标。

## Goals / Non-Goals

**Goals:**

- 用一次可部署的纵向实现打通模拟/DJI 无人机、实时遥测、Tasks 编排、直播/媒体、外部疑似违建识别、案件协作和时空回放。
- 保持厂商协议、媒体传输、算法服务和业务编排之间的边界，后续接入 MAVLink、ROS 2、地面机器人或新算法时不修改核心业务契约。
- 在所有异步边界采用至少一次投递、幂等消费和可审计状态机，不以“恰好一次”的不现实假设驱动物理设备。
- 让实时界面在依赖降级时明确显示数据新鲜度和不可用原因。
- 对所有项目资源执行服务端租户授权，并通过 AI SDK 把时空大模型限制在用户 scope 内的查询、草案和受保护调度控制面。
- 形成以项目地图总览为默认入口、以统一时间线串联设备轨迹、任务、媒体、算法运行和案件的 UI 信息架构。

**Non-Goals:**

- 不在首期自研视频编解码、云端飞控、通用 SLAM、三维重建或模型训练框架。
- 不以大模型替代 Task 条件求值器、任务状态机、设备适配器或安全策略。
- 不交付生产可用的 ROS 机器人适配器、自动机器人派单、空地目标融合、跨地域多活、离线边缘自治或特征级实时协同感知；这些能力通过协议和数据模型预留扩展点。
- 不将 UOM 对接认定为已满足合规，仅保存相关元数据并提供策略校验点。

## Decisions

### 1. 保持 Web 控制面与 Go 异步数据面的职责分离

- Next.js 负责用户会话、项目授权、同步 CRUD、预签名媒体访问、任务/案件操作 API 和 Web UI。
- Go worker 负责设备适配器运行、遥测归一化、任务调度、命令投递、状态评估、Task 条件处理、媒体派生调度、保留清理和报告生成。
- 外部设备回调优先进入 Go ingest endpoint；需要用户会话的同步操作始终通过 Next.js。worker 只接受内部服务身份或已验证厂商签名，不接受浏览器会话作为信任来源。

选择理由：与仓库现有所有权一致，并避免把长连接和后台重试塞进 Next.js 请求生命周期。备选方案是全部使用 Next.js Route Handler；实现初期更少代码，但无法稳定承载适配器长连接和调度循环，故不采用。

### 2. 使用 PostgreSQL 事务数据 + outbox/job queue，暂不强制新增独立消息集群

- 业务写入与待处理事件在同一事务中写入 `outbox_events`；worker 使用 `FOR UPDATE SKIP LOCKED` 领取任务，失败按 `available_at`、`attempts` 和死信状态重试。
- `LISTEN/NOTIFY` 只作为低延迟唤醒信号，事实来源仍是 outbox 表；丢失通知不会丢失任务。
- 浏览器实时更新首期通过 Next.js SSE Route Handler，按项目订阅数据库通知并在连接建立时先读取快照；SSE 断开后客户端指数退避重连并基于游标补齐。
- 当高频遥测量达到单库写入或 SSE 扇出阈值时，可在相同事件 envelope 外接 NATS JetStream/Kafka；核心消费者仍以事件标识幂等。

选择理由：当前部署只有 PostgreSQL，事务 outbox 能最小化首期运维面并满足可恢复性。备选方案是立即引入 NATS JetStream；吞吐和扇出更好，但增加部署、授权、监控和双写一致性复杂度，留到容量数据证明需要时再引入。

### 3. 设备 adapter 只统一领域语义，不假定底层协议一致

统一 envelope 至少包含：`schema_version`、`event_id`、`adapter_id`、`project_id`、`external_device_id`、`event_type`、`captured_at`、`received_at`、`sequence`、`payload`、`signature_context`。下行命令包含 `command_id`、`idempotency_key`、`capability_code`、`parameters`、`deadline` 和 `safety_context`。

- `simulator` 适配器是验收基准：可产生无人机与模拟地面移动设备的发现、心跳、位姿轨迹、电量、媒体引用、ACK/NACK、超时、乱序和掉线。
- DJI 适配器封装厂商 API、回调签名、设备拓扑、直播和任务命令映射，但只暴露统一 envelope。
- `DeviceType` 和 capability 决定设备可执行的任务与 UI 符号；无人机和未来 ROS 机器人共享 `Pose` / `Track` 结构，但不会为了“统一”而抹平起降、导航、云台等能力差异。
- MAVLink、ROS 2、MQTT、GB28181 等名称只作为未来 adapter 类型与能力声明预留。未实现的类型必须返回明确的“不支持”，不得表现为已接入。
- 适配器凭据只保存秘密管理器引用；数据库不保存明文。

选择理由：模拟适配器让全链路测试不依赖真实硬件或厂商租户。备选方案是直接在业务服务中调用 DJI SDK；会将厂商语义扩散到任务和 UI，不采用。

### 4. 使用 PostGIS 建立“原始值 + 标准值”的时空模型

- 所有时间列使用 `timestamptz`；观测同时保存 `captured_at` 与 `received_at`，以 `(source_id, event_id)` 唯一去重。
- 地图检索使用标准 WGS84 `geometry(PointZ, 4326)` / 其他 geometry，并为项目、采集时间和 GiST 空间列建立组合索引。
- 位姿额外保存方向四元数、速度、水平/垂直/姿态精度、原始 CRS、垂直基准和转换版本。需要局部融合时派生项目 ENU 坐标，不覆盖原值。
- 高频遥测表按月或按时间范围分区；连接与设备表只保存最新快照，历史事实保存在 append-only 表。
- 所有融合输入要求可解析 CRS、位姿和校准版本；质量不足的数据仍可查询，但标记为不可融合。

选择理由：PostGIS 与现有 PostgreSQL 一致，足以支持 MVP 空间查询和地图。备选方案是独立时序数据库/Elasticsearch；首期会造成多存储一致性与租户授权重复实现，不采用。

### 5. 数据库按事实、当前投影和业务流程分层

建议新增或扩展以下结构：

- 接入：`device_adapters`、`device_external_identities`、`device_connections`、`device_telemetry`、扩展 `devices` 与 `device_capabilities`。
- 时空：`coordinate_references`、`sensor_calibrations`、`observations`、`poses`。
- 媒体：`live_streams`、扩展 `assets`、`asset_derivatives`、`evidence_links`、`retention_holds`。
- 任务：扩展 `tasks`、`task_versions`、`task_steps`、`task_triggers`、`task_runs`、`task_run_steps`，并复用 `device_commands`、`command_attempts`、`approvals`。
- 算法：`algorithm_providers`、`algorithm_definitions`、`algorithm_definition_versions`、`algorithm_runs`、`algorithm_run_attempts`、`detections`、`detection_groups`。
- 案件与治理：扩展 `issues`、`issue_events`、`issue_links`、统一 `issue_assignees`、`project_permissions`、`safety_policy_versions`、`idempotency_records`、`audit_events`、`outbox_events`、`generated_reports`。既有 `event_rules`、`event_rule_versions`、`perception_events`、`event_feedback` 与 `alert_automation_*` 表停止产生新的主动编排记录，仅为升级兼容和历史审计保留。

所有项目级表直接包含 `project_id`，即使可通过父表推导，以便数据库约束、索引和授权查询不依赖多跳关系。外键应验证父子资源同项目；若 PostgreSQL 普通外键无法直接表达，则使用包含 `(project_id, id)` 的唯一键和复合外键。

新运行时把算法运行、检测和 Task 条件步骤输出作为机器事实，不再经过独立的业务事件生命周期。`issues` 是用户可见的案件，保存需要人员或 Copilot 跟踪的工作项；任务的 `issue.create-or-update` 步骤以幂等键创建或更新案件，并通过 `issue_links` 关联算法运行、检测、条件步骤、任务运行和资产。`issue_events` 统一承载评论与状态活动，`issue_assignees` 以 `user` / `agent` 主体类型统一人员和 Copilot 指派。历史 `perception_events` 可只读展示或迁移关联，但新案件不得依赖其自动化状态机。

### 6. Tasks 是唯一业务编排层，运行采用数据库持久化状态机和命令账本

- `task_versions` 固化可发布的任务模板；模板包含类型化输入、手动/定时/API/Webhook/Copilot 委派触发器、顺序步骤、步骤依赖、输出模式、条件表达式、重试与失败策略。产品和 API 始终称其为 Tasks/任务，不引入另一套编排产品名。
- MVP 执行模型为单个 Task Run 内的顺序步骤和条件分支。每一步声明 `uses` 能力，例如 `device.command`、`device.collect`、`algorithm.run`、`issue.create-or-update`、`copilot.run`、`report.generate`，并以 `steps.<key>.outputs` 向后续步骤提供不可变输出；数据模型保留未来扩展多 Job/DAG 的依赖字段，但首期 UI 不承诺矩阵或任意并行图。
- 设备、连接器、算法定义、Copilot 和报告器只注册静态资源或可调用能力，不自行保存跨领域触发规则。定时巡逻、采集后推理、阈值判断、案件创建和 AI 自动分析都必须能定位到一个已发布任务版本及其运行。
- `task_runs.input_snapshot_json` 保存触发来源、选定版本和类型化输入；`task_run_steps` 保存解析后的输入、条件结果、输出引用、尝试与失败。每个状态转换在事务中校验预期版本，更新当前状态，追加审计并写入 outbox。
- 条件步骤只读取已保存的结构化输出，使用受限确定性表达式比较类别、置信度、数量、空间关系或状态，不执行任意代码。`issue.create-or-update` 使用项目、任务、条件和业务键幂等，避免重复投递形成案件风暴。
- 调度器每次只推进一个可执行步骤。物理动作先创建 `device_commands`，通过安全策略后进入 `dispatchable`，收到匹配 ACK/NACK/结果后推进步骤；可安全重试的命令复用同一幂等键，状态未知的非幂等命令进入 `paused` 等待人工处置。
- `copilot.run` 是显式任务步骤，其调用授权随任务版本保存并在每次运行时重新校验；Copilot 如需调用设备、算法或报告能力，仍通过任务工具和领域 API，不建立第二套执行器。紧急停止继续使用独立高优先级路径，但仍记录命令与确认。

选择理由：把所有跨资源流程集中到 Tasks，用户可以从一次运行完整解释“为什么调用设备、为什么运行算法、为什么创建案件或调用 Copilot”，同时复用既有状态机和命令账本。备选方案是在算法、独立规则、案件和 AI 设置中分别配置自动化；会形成隐藏触发链和重复权限模型，不采用。首期也不引入 Temporal 或任意代码执行器，以控制部署面与安全边界。

### 7. 媒体采用对象存储，直播采用外部流端点

- `assets` 只保存对象键、媒体类型、大小、校验和、时间空间范围和版本；上传使用受项目授权的预签名 URL，完成回调后再次校验对象元数据。
- 本地开发使用 S3 兼容服务，生产可接云对象存储。缩略图、转码片段等均作为带 `derived_from_asset_id` 的独立资产版本。
- `live_streams` 保存直播控制状态和短期播放 locator；厂商直播、RTMP/HLS/WebRTC 转换由适配器或外部媒体网关承担，长期播放凭据不入库。
- `evidence_links` 使用受控目标类型与目标标识，并固化资产版本、校验和、视频偏移和创建者。已发布报告通过保全阻止证据对象被清理。

选择理由：数据库不适合保存大型二进制和直播流。备选方案是首期部署统一媒体服务器并转码所有输入；在输入协议与规模未确定时成本过高。

### 8. 算法服务采用“统一运行契约 + 多协议 adapter”

平台不把某个厂商的推理 API 固化为业务接口。`algorithm_providers` 保存项目可用的服务连接，核心字段为 `type`、`base_url`、`secret_ref`、认证方式、允许的额外 header、超时、并发和速率限制；`algorithm_definitions` / version 保存能力名称、输入资产要求、协议参数和输出映射。API key 只能写入秘密管理器，普通 API 与 UI 仅返回掩码和 `secret_ref`。

统一运行契约为：

1. 业务创建不可变 `algorithm_run`，引用算法定义版本、输入资产版本、任务/设备/时空上下文与幂等键。
2. worker 按 provider `type` 选择 adapter，使用 S3 预签名 URL 传递图像或视频片段，避免将大文件 base64 塞入数据库或队列。
3. adapter 支持同步响应、异步轮询或签名 callback，并转换为平台 canonical result。
4. canonical result 至少包含 provider/model/version、标签、置信度、像素 bbox/polygon/mask 引用、可选地理 geometry、时间、输入资产引用、原始结果对象键与 mapping 版本。
5. 原始外部结果只存对象存储并校验哈希；映射失败保留原始结果和诊断，不能伪造成功 detection。

协议类型首期定义 `http-json`、`kserve-v2`、`ogc-processes`、`ai-sdk`。`http-json` 是 MVP 必须可运行的通用路径；其他类型按照同一 adapter 接口实现或在未启用时明确返回能力不可用。KServe V2 解决张量级推理调用，OGC API Processes 解决地理处理作业，OpenAPI/JSON mapping 解决非标准业务字段；这些标准都不替代平台自己的违建、轨迹和证据语义。

出站调用默认只允许 HTTPS 和项目/部署级域名 allowlist，解析后拒绝 loopback、link-local、云元数据地址与未授权私网；重定向需重新校验。callback 使用随机 token 与签名，且必须匹配 run 和 provider。worker 对每次 attempt 记录延迟、状态码、错误类别、请求摘要哈希和计费元数据，不记录秘密或完整敏感请求。

选择理由：业务面对稳定的 `algorithm_run` 与 canonical result，外部服务差异集中在 adapter 和 mapping。备选方案是要求所有服务改造成单一 KServe endpoint；这会排除异步地理处理和现有 HTTP 视觉服务，故不采用。

### 9. 疑似违建链路以“机器线索”而非法律结论建模

- 首期从巡检媒体触发 `suspected-construction` 算法定义，保存建筑新增/扩建等标签、置信度和像素 polygon。
- 当资产具有有效无人机位姿、相机标定和地理参考时，worker 尝试投影为地理 polygon，并保存方法、误差和版本；条件不足时仍可形成图像级线索，但地图不得展示虚假精确位置。
- 同一项目内按空间重叠、时间窗口和来源资产聚合为 `detection_group`；Task 的确定性条件步骤直接读取算法运行与检测输出，条件满足时通过幂等步骤创建或更新案件，避免每帧一个案件。
- UI 和报告统一称“疑似违建”，保留原图、框选/分割结果、时间、位置质量、模型版本和人工反馈。最终是否违建需要规划底图、审批数据或人工核验，MVP 不作法律判断。

选择理由：先验证巡检闭环，同时避免把视觉变化识别错误包装成执法结论。备选的跨期正射影像变化检测可作为后续算法定义接入，不阻塞当前架构。

### 10. 项目默认入口采用地图工作台 + 同步时间线

- 项目级 layout 固定侧边栏：`总览`、`实时作业`、`任务`、`设备`、`连接器`、`案件`、`算法服务`、`智能体`、`数据资产`、`设置`。访问 `/projects/:projectId` 重定向或渲染 `总览`，而不是空白列表页。
- 总览由顶部项目状态与时间模式栏、中央 MapLibre 地图、右侧上下文详情抽屉、底部可折叠时间线组成；图层/筛选面板可覆盖在地图左侧，但不取代主导航。
- 地图按 layer 展示无人机、机巢、未来 ROS 机器人、实时/历史轨迹、任务区域与航线、媒体采集点、疑似违建 geometry 和案件。不同设备用不同图标和颜色，轨迹均读取统一 `Pose` / `Track` view model。
- 时间线不是单一日志列表，而是按设备轨迹、任务步骤、媒体、算法运行、检测和案件分轨。选择时间点或时间范围会同步地图位置/轨迹裁剪、右侧详情、媒体帧和案件列表；点击地图对象也反向定位时间线。
- `实时` 模式跟随最新游标并允许有权限的任务操作；`历史回放` 模式固定查询窗口且 API/UI 均禁止控制。用户拖动时间线后自动退出跟随，可显式回到实时。
- 页面首次加载由服务端读取项目授权和当前快照；客户端随后建立带项目和最后游标的 SSE。每个项目事件具有单调数据库游标，重连补发有限增量，超出窗口则重载快照。

选择理由：地图承载“在哪里”，时间线承载“何时发生且前后如何关联”，两者共同作为时空数据的主索引。备选方案是为任务、视频和案件分别做孤立页面；会丢失跨域上下文，不采用。SSE 适合首期服务器单向推送；达到连接与扇出阈值后可迁移专用实时网关。

### 11. AI SDK 智能体通过服务端 scope 和白名单工具访问领域服务

- Web 使用 Vercel AI SDK `ai` 包与 provider packages，通过 `createProviderRegistry` 或等价注册表按部署配置选择大模型，不将单一模型 SDK 扩散到领域服务。
- 每次会话由服务端创建 `AgentExecutionContext { userId, teamId, projectId, sessionId }`。模型输入、tool args、提示词中的 `projectId` 或 `userId` 均不作为授权依据。
- 工具分为只读查询、写入草案和受保护调度三类。只读工具覆盖设备、任务、案件、资产和当前地图上下文；草案工具覆盖任务/报告/案件评论草案；受保护调度工具只能创建、调用或请求启动 Tasks，并由 Tasks 进入领域 API 的执行入口。
- tool call 执行前按 context 重新验证团队成员关系和项目 permission；排队后的 worker 在真正执行前再次鉴权并校验 scope 快照/有效期。用户已离组、权限撤销或项目不匹配时失败关闭。
- 模型永远不直接调用 DJI/设备 adapter。启动任务需依次经过参数 schema、任务版本、安全预检、审批策略、幂等记录、命令账本和设备 ACK；每一步都可审计。
- 工具结果返回稳定资源引用、采集时间、质量和有限数据；秘密、临时签名 URL 和完整原始视频不写入对话历史。报告草稿记录模型、提示模板、工具调用与证据版本。

选择理由：时空大模型可以真正查询和调度平台能力，但授权和物理安全仍由确定性领域层掌控。备选方案是只允许生成草案，安全面更小但不能满足调度目标；通用 shell/MCP 直连设备则风险不可接受。

### 12. 案件是协作对象，Copilot 只能由 Tasks 或案件交互显式调用

- 用户侧不展示“告警事件”工作台。算法与检测事实经过 Task 条件后创建或更新 `issue`，案件使用项目内递增编号、标题、描述、开放/关闭状态、优先级、标签、证据、关联任务和统一活动时间线。
- `issue_events` 的 `comment` 类型保存用户或 Copilot 评论；状态、指派、标签和关联变化也作为活动记录展示。正文只保存稳定资源引用，不保存永久凭据或临时媒体 URL。
- `issue_assignees` 使用统一主体模型关联用户或项目 Agent。把案件指派给 Copilot，或有 `agent:use` 权限的用户在评论正文中提及 `@copilot`，都会幂等创建绑定该案件、评论、发起用户和当前项目的 Agent Session/Job。
- Copilot 先以案件评论发布已接收/进行中/失败或最终结果；如需生成报告、后续任务或处置方案，则保存可审阅草案并在评论中链接。涉及设备控制或其他受保护动作时，只能请求启动 Task，仍需权限、预检、审批、确认和命令 ACK。
- 自动 Copilot 只来自已发布任务中的 `copilot.run` 步骤；临时 Copilot 只来自案件提及/指派或智能体聊天。项目设置不提供自动 AI 开关或后处理模式，算法完成、检测保存或案件创建本身不会隐式调用模型。
- AI 服务失败不得回滚任务已经保存的采集数据、算法输出、检测、条件结果或案件；每次调用记录任务/评论触发源、scope、模型、提示模板、工具调用、证据版本、产物和失败信息。

选择理由：自动化授权落在可版本化、可审计的 Task 定义或一次明确的案件交互上，既覆盖定时自动处理和临时协助，也避免项目级模式产生难以解释的隐藏行为。

### 13. 权限使用角色默认值 + 项目级显式授权

- 保留 `team_members.role`；owner/admin 获得项目管理默认权限，member 默认只读现有项目。
- `project_permissions` 为 member 增加 `issue:handle`、`issue:assign`、`mission:operate`、`mission:approve`、`algorithm:manage`、`agent:use` 等显式权限，不改变其团队角色。迁移期可把旧 `event:handle` 映射为 `issue:handle`，但新 UI/API 不继续扩散旧名称。
- 所有查询以授权用户和目标项目开始，再读取子资源。内部 worker 操作使用服务身份并要求事件中包含已验证的 `project_id`。
- 高风险策略可要求请求者与批准者不同；审批表使用唯一约束防止重复批准。

备选方案是在首期引入通用 ABAC/策略引擎；表达力更强，但当前权限规模不足以抵消复杂性。领域权限代码应集中，后续可替换实现。

### 14. 可观测性与验收指标

- 结构化日志统一携带 `request_id`、内部 `event_id`、`project_id`、`device_id`、`task_run_id`、`issue_id` 和 `command_id`，敏感字段先脱敏。
- 指标至少覆盖适配器连接、摄取速率/延迟/拒绝、outbox 积压、命令 ACK 延迟、任务与步骤状态、直播状态、算法调用延迟/错误/积压、案件创建延迟、SSE 连接、Copilot 鉴权拒绝和报告失败。
- MVP 目标：在基准负载下，遥测从接收到地图/时间线更新 P95 ≤ 2 秒，canonical detection 经任务条件到案件可见 P95 ≤ 5 秒（不含外部算法耗时）；所有受保护调度具有审计记录，重复事件、算法 callback、任务步骤和命令测试不产生重复副作用。
- 性能目标通过模拟适配器负载测试验证；它们是部署基准而非对不稳定外部网络的端到端保证。

## Risks / Trade-offs

- [PostgreSQL 同时承担业务、时空、outbox 和实时游标可能形成瓶颈] → 分区高频表、批量摄取、限制 SSE 补发窗口、监控积压，并以事件 envelope 为边界预留独立总线。
- [不同设备时钟、坐标和标定误差导致轨迹错位或错误地理投影] → 双时间、质量字段、转换/校准版本和可视化精度提示；质量不足时只展示图像级检测。
- [厂商 API 或直播能力受许可和网络限制] → simulator 作为必达验收路径，厂商适配器声明能力与降级原因，核心流程不依赖单一厂商在线。
- [SSE 在无状态多实例下的数据库连接与扇出成本] → 每实例共享项目订阅、限制每用户连接数；达到阈值后迁移到专用 realtime gateway。
- [任务取消或紧急停止时设备失联] → 明确“安全状态未知”，高优先级重试与现场处置提示；平台不宣称未确认的物理安全结果。
- [对象存储与数据库写入无法原子提交] → 采用上传意图、对象校验、完成记录和孤儿对象清理的 saga，不发布未完成资产。
- [可配置 `base_url` 和 header 形成 SSRF/秘密泄露面] → HTTPS/域名 allowlist、DNS 解析后地址检查、重定向复验、秘密引用、日志脱敏和 callback 签名。
- [外部算法输出格式漂移或长时间不可用] → 版本化 mapping、契约测试、原始结果保全、熔断/退避和任务步骤失败可见；不得将解析失败当作“无违建”。
- [智能体幻觉、越权或提示注入] → 事实回答引用工具结果，服务端固定 scope，tool 与 worker 双重鉴权，模型不直连设备 adapter，受保护调度走既有审批和命令账本。
- [Tasks 统一编排后单个任务定义过于复杂] → MVP 限制为类型化顺序步骤与受限条件表达式，提供版本化模板和逐步输出检查；多 Job/DAG 在运行证据表明需要后再开放。
- [一次变更包含较多领域] → 按 tasks.md 的纵向里程碑实施，每个里程碑可独立启用；功能开关默认关闭未完成能力。

## Migration Plan

1. 先部署只增不删的数据库迁移：启用 PostGIS，创建新表、索引、复合约束和 outbox；对现有表仅增加带默认值或可空列。
2. 部署仍关闭新功能的 Web/worker，运行 schema 与授权回归；worker 能安全忽略未知事件类型。
3. 启用 simulator 和项目总览功能开关，完成地图、时间线、轨迹、任务、直播占位、资产和回放的端到端验收。
4. 对内部项目启用对象存储、`http-json` 算法 provider，并发布“巡检—采集—算法—条件—案件”任务模板；验证 SSRF、秘密、callback、条件幂等和案件去重测试。
5. 启用 AI SDK 聊天、案件 `@copilot`/指派和任务 `copilot.run` 步骤，再单独启用受保护任务调度；验证跨项目、撤权后排队和提示注入测试。
6. 接入 DJI 沙箱/测试组织，验证签名、设备身份映射、直播与命令能力，再对单个试点项目开启。
7. 在达到指标和安全演练通过后扩大项目范围；保留每项目设备下行、外部算法开关。回滚 AI 时停用含 `copilot.run` 的任务版本并停止新的 Copilot Job，不影响任务事实和案件人工协作。

回滚时先关闭项目功能开关与适配器下行命令，停止 worker 新任务，等待或人工终止活动运行，再回滚应用版本。数据库新增结构保持不删除，旧应用忽略新增表列；若新 worker 事件已写入 outbox，回滚版本必须跳过未知事件并保留供重新部署处理。对象存储不立即删除，避免回滚丢失证据。

## Open Questions

- 生产对象存储和秘密管理器的具体供应商可在部署阶段选择；接口、测试和数据模型不依赖供应商。
- DJI 首个试点使用哪种账号层级、机型和直播授权需要在接入任务开始前确认，但 simulator 与核心 MVP 不受阻塞。
- 项目标准底图供应商和中国境内坐标展示策略由部署地区决定；数据库保留原始 CRS 和 WGS84 标准值以支持适配。
