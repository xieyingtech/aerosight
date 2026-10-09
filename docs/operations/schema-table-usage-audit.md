# 全量表用途审计与清理结果

清理前 115 张业务物理表；0090 后 91 张；0091 后 87 张；0092 合并三组活跃一对一扩展后 **84 张**（83 个业务表/分区父表 + 1 个默认遥测分区）。不含迁移账本和 PostGIS 扩展表。另有 12 个不存储数据的兼容视图。此报告覆盖原有全部表及新增的统一作业表。

审计先检查 SQL 读写和 sqlc 方法的实际生产调用，再核对 HTTP 路由、runtime 注册、事件来源、唯一键/RETURNING 门禁与外键。测试造数、生成模型、历史迁移、查询声明以及 cmd 维护工具都不单独算线上业务使用。`python scripts/audit-schema-usage.py` 可重新导出依赖线索至 `.build/table-audit.json`；脚本不能证明一个处理器已经接入生产，删除判断仍需检查入口链。

## 本轮补漏

| 原表 | 实际情况与处理 |
|---|---|
| `asset_upload_intents` | 无当前创建、完成或过期处理写入；媒体读取仅为获取文件名 LEFT JOIN。原文件名迁到 assets.metadata_json.uploadFileName；全部旧意向归档，含未绑定素材的 pending 行。 |
| `evidence_links` | 无当前写入；媒体访问只查已发布标志。完整版本、校验和、时间偏移、目标与发布状态归档；已发布素材保留敏感下载与删除/标志保护。正式报告仍使用 generated_report_evidence。 |
| `event_feedback` | 无当前写入；旧事件详情只读历史。快照迁到 perception_events.legacy_feedback_json，旧 API 保留 ID、值、原因、时间及当前用户名；正式反馈继续用 issue_feedback。 |
| `asset_derivatives` | 仅 media.SaveDerivative 写、无业务读取。runtime 注册 asset.available 消费者，但当前素材导入没有发布该事件。关系迁到 assets 的项目约束来源 FK 与 derivative_type，生成器参数保留 metadata，原有全部关系归档。注册处理器继续支持旧队列/兼容事件，重试不重复素材。历史多来源选择最早关系为主来源，其他原边仍可在归档恢复。 |

## 0092 活跃一对一扩展合并

| 原物理表 | 运行用途与合并方式 |
|---|---|
| `device_command_protocol_correlations` | 协议事务、回执与超时仍被运行时消费；迁到 device_commands.protocol_*，与业务 status 分开，保留旧 ID/时序、唯一键和项目外键；原名为过滤视图。 |
| `connector_asset_access_refs` | 司空远程文件定位与受控解密仍使用；迁到 assets.remote_* 私有列，原资产 ID 的加密 AAD 不变，摘要/密文不会出现在公开素材响应；原名为过滤视图。 |
| `poses` | 轨迹、回放和空间工具仍使用；迁到 observations.pose_*，保留独立设备/时间、PointZ、姿态/速度/精度以及时空索引，新位置观测一次 INSERT；原名为过滤视图。 |

三个兼容视图不保留副本。删除扩展或其来源只清空对应扩展，保留父业务行；跨项目 FK、重复事务/访问引用拒绝仍在数据库生效。

0090 遗留的 retention/safety/evidence 三个无使用触发器函数一并删除。历史 migration 文件不改写；维护脚本的旧结构断言先在 0089 上构造并验证旧行为，再对有数据的库升级到当前版本；当前快照明确要求旧表不存在。浏览器巡检夹具和回滚演练使用新结构。

## 保留表逐项依据

证据列给出一个代表性生产路径，不是访问总量；SQLc 项目同时列出实际调用方。没有单独 SELECT 的状态门禁、自动分区、迁移种子目录及仍生效的授权表由用途列说明。

| 表 | 实际用途与保留理由 | 写入证据 | 读取/消费证据 |
|---|---|---|---|
| `agent_drafts` | 智能体待审核草稿及证据快照，与正式报告分离。 | [processor.go:284](../../apps/server/internal/agent/processor.go) | [issue_reads.sql:98](../../apps/server/internal/database/queries/issue_reads.sql) `ReadIssueDrafts` → [issue_reads.go:68](../../apps/server/internal/httpapi/issue_reads.go) |
| `agent_messages` | 会话消息与工具调用历史。 | [agent_sessions.sql:17](../../apps/server/internal/database/queries/agent_sessions.sql) `AppendChatMessage` → [agent_sessions.go:177](../../apps/server/internal/httpapi/agent_sessions.go) | [agent_sessions.sql:5](../../apps/server/internal/database/queries/agent_sessions.sql) `ListChatMessages` → [agent_sessions.go:73](../../apps/server/internal/httpapi/agent_sessions.go) |
| `agent_sessions` | 会话归属、摘要、案件和任务关联。 | [assessment_step.go:87](../../apps/server/internal/agent/assessment_step.go) | [assessment_worker.go:52](../../apps/server/internal/agent/assessment_worker.go) |
| `agent_tool_jobs` | 智能体异步执行、研判及恢复检查点。 | [assessment_step.go:91](../../apps/server/internal/agent/assessment_step.go) | [assessment_worker.go:34](../../apps/server/internal/agent/assessment_worker.go) |
| `agent_write_approvals` | 智能体写操作的审批绑定与一次性授权；不能用普通聊天消息替代。 | [agent_workflow_tools.go:161](../../apps/server/internal/httpapi/agent_workflow_tools.go) | [agent_sessions.go:83](../../apps/server/internal/httpapi/agent_sessions.go) |
| `agents` | 项目智能体配置、模型和工具策略。 | [issue_writes.sql:36](../../apps/server/internal/database/queries/issue_writes.sql) `EnsureIssueCopilot` → [issue_writes.go:221](../../apps/server/internal/httpapi/issue_writes.go) | [assessment_step.go:74](../../apps/server/internal/agent/assessment_step.go) |
| `ai_providers` | 平台模型服务配置与加密凭据。 | [ai_defaults.go:95](../../apps/server/internal/httpapi/ai_defaults.go) | [processor.go:203](../../apps/server/internal/agent/processor.go) |
| `algorithm_callback_receipts` | 异步回调去重及结果接收状态。 | [callback.go:278](../../apps/server/internal/algorithm/callback.go) | [callback.go:304](../../apps/server/internal/algorithm/callback.go) |
| `algorithm_definition_versions` | 不可变算法版本；历史运行必须固定版本。 | [algorithm_definitions.sql:14](../../apps/server/internal/database/queries/algorithm_definitions.sql) `RetireAlgorithmConfigurations` → [algorithm_definitions.go:217](../../apps/server/internal/httpapi/algorithm_definitions.go) | [callback.go:198](../../apps/server/internal/algorithm/callback.go) |
| `algorithm_definitions` | 项目算法定义及当前版本指针。 | [algorithm_definitions.sql:1](../../apps/server/internal/database/queries/algorithm_definitions.sql) `CreateAlgorithmDefinition` → [algorithm_definitions.go:198](../../apps/server/internal/httpapi/algorithm_definitions.go) | [callback.go:198](../../apps/server/internal/algorithm/callback.go) |
| `algorithm_providers` | 算法服务地址、协议及加密凭据。 | [admin_algorithm_providers.go:138](../../apps/server/internal/httpapi/admin_algorithm_providers.go) | [callback.go:198](../../apps/server/internal/algorithm/callback.go) |
| `algorithm_run_attempts` | 图像/异步算法请求重试历史；视频普通帧诊断已改为对象存储。 | [processor.go:68](../../apps/server/internal/algorithm/processor.go) | [algorithm_runs.sql:49](../../apps/server/internal/database/queries/algorithm_runs.sql) `ReadAlgorithmRunAttempts` → [algorithm_runs.go:52](../../apps/server/internal/httpapi/algorithm_runs.go) |
| `algorithm_runs` | 算法运行状态、原素材、采样配置与视频结果摘要。 | [callback.go:268](../../apps/server/internal/algorithm/callback.go) | [callback.go:198](../../apps/server/internal/algorithm/callback.go) |
| `approval_requests` | 任务/控制操作的审批请求与所需票数。 | [flighthub_flight_launch.go:232](../../apps/server/internal/httpapi/flighthub_flight_launch.go) | [control_command.go:233](../../apps/server/internal/flighthub/control_command.go) |
| `approvals` | 逐人审批决定；一张请求可对应多个决定。 | [mission_control.sql:4](../../apps/server/internal/database/queries/mission_control.sql) `ApproveMissionControlRun` → [mission_control.go:104](../../apps/server/internal/httpapi/mission_control.go) | [mission_audit.sql:11](../../apps/server/internal/database/queries/mission_audit.sql) `GetMissionAuditApproval` → [mission_audit.go:93](../../apps/server/internal/httpapi/mission_audit.go) |
| `assets` | 素材目录、对象存储指针、来源、拍摄时间、派生关系和私有远程访问引用；公开响应显式选列。 | [flight_alert_projector.go:337](../../apps/server/internal/flighthub/flight_alert_projector.go) | [processor.go:171](../../apps/server/internal/agent/processor.go) |
| `audit_events` | 业务/平台审计、权限操作与迁移历史归档。 | [inspection_step.go:202](../../apps/server/internal/issue/inspection_step.go) | [mission_audit.sql:5](../../apps/server/internal/database/queries/mission_audit.sql) `GetMissionAuditRequest` → [mission_audit.go:76](../../apps/server/internal/httpapi/mission_audit.go) |
| `command_attempts` | 指令发送重试与协议回执检查点。 | [command_dispatch.go:180](../../apps/server/internal/dji/command_dispatch.go) | [command_dispatch.go:180](../../apps/server/internal/dji/command_dispatch.go) |
| `connector_capability_snapshots` | 动态探测能力与有效期，不能仅用静态 connector 定义替代。 | [resources.go:437](../../apps/server/internal/connector/resources.go) | [resources.go:475](../../apps/server/internal/connector/resources.go) |
| `connector_control_sessions` | 司空主控权租约、凭据及释放状态；区别于普通指令作业。 | [control_session.go:174](../../apps/server/internal/flighthub/control_session.go) | [control_session.go:90](../../apps/server/internal/flighthub/control_session.go) |
| `connector_definitions` | 连接器静态目录由迁移种子维护，运行时据此校验类型和能力；没有用户写接口是预期行为。 | —（见用途说明） | [lease.go:56](../../apps/server/internal/connector/lease.go) |
| `connector_jobs` | 八类司空作业的唯一物理存储；job_type 限定状态、字段和幂等。 | [device_admin_action.go:128](../../apps/server/internal/flighthub/device_admin_action.go)（经 `connector_device_admin_jobs` 视图） | [device_admin_action.go:83](../../apps/server/internal/flighthub/device_admin_action.go)（经 `connector_device_admin_jobs` 视图） |
| `connector_open_model_uploads` | 开放模型上传的凭据会话和回调生命周期；区别于一次请求作业。 | [open_model_upload.go:113](../../apps/server/internal/flighthub/open_model_upload.go) | [open_model_upload.go:120](../../apps/server/internal/flighthub/open_model_upload.go) |
| `connector_remote_resources` | 司空远程航线、飞行、告警、模型的统一缓存和私有巡检证据。 | [resources.go:207](../../apps/server/internal/connector/resources.go) | [airsense_projector.go:82](../../apps/server/internal/flighthub/airsense_projector.go) |
| `connector_resource_sync_states` | 不同远程资源流的游标、失败次数和同步租约。 | [resources.go:300](../../apps/server/internal/connector/resources.go) | [resources.go:341](../../apps/server/internal/connector/resources.go) |
| `connector_sync_runs` | 实际同步器和失败处理器持续写批次结果；device_external_identities.last_sync_run_id 外键关联来源批次。无当前列表页面，保留来源追溯。 | [scheduler.go:101](../../apps/server/internal/connector/scheduler.go) | —（见用途说明） |
| `coordinate_references` | 输入坐标系与转换定义，观测/遥测 FK 引用；不是空间点副本。 | [ingestor.go:167](../../apps/server/internal/telemetry/ingestor.go) | [ingestor.go:160](../../apps/server/internal/telemetry/ingestor.go) |
| `detection_groups` | 识别结果聚合组，算法投影及司空事件都使用。 | [airsense_projector.go:103](../../apps/server/internal/flighthub/airsense_projector.go) | [repository.go:75](../../apps/server/internal/perception/repository.go) |
| `detections` | 结构化识别目标、置信度和空间投影；现在直接带 group_id。 | [repository.go:49](../../apps/server/internal/perception/repository.go) | [processor.go:171](../../apps/server/internal/agent/processor.go) |
| `device_adapters` | 适配器配置、凭据、运行租约及巡检策略。 | [lease.go:56](../../apps/server/internal/connector/lease.go) | [registry.go:35](../../apps/server/internal/adapter/registry.go) |
| `device_capabilities` | 设备实际上报/探测的能力，与用户授权不同。 | [projection.go:238](../../apps/server/internal/dji/projection.go) | [flighthub_realtime_operations.go:131](../../apps/server/internal/httpapi/flighthub_realtime_operations.go) |
| `device_capability_grants` | 设备级显式用户授权，被命令策略与快照读取；当前未提供管理写接口，不能删除其授权语义。 | —（见用途说明） | [device_commands.sql:6](../../apps/server/internal/database/queries/device_commands.sql) `LockDeviceCommandGrants` → [device_commands.go:159](../../apps/server/internal/httpapi/device_commands.go) |
| `device_commands` | 设备指令业务状态及独立协议事务/回执状态；保留协议幂等、超时和原关联 ID。 | [command_dispatch.go:136](../../apps/server/internal/dji/command_dispatch.go) | [command_dispatch.go:51](../../apps/server/internal/dji/command_dispatch.go) |
| `device_connections` | 心跳连接与在线状态投影。 | [projector.go:106](../../apps/server/internal/heartbeat/projector.go) | [projector.go:153](../../apps/server/internal/heartbeat/projector.go) |
| `device_connector_bindings` | 设备与多个连接器的受管/观测角色、路由优先级。 | [sync.go:355](../../apps/server/internal/connector/sync.go) | [resources.go:381](../../apps/server/internal/connector/resources.go) |
| `device_external_identities` | 内部设备与外部标识映射，跨批同步保留身份。 | [registry.go:35](../../apps/server/internal/adapter/registry.go) | [resources.go:381](../../apps/server/internal/connector/resources.go) |
| `device_latest_telemetry` | 每设备当前遥测投影，控制前置校验无需扫描历史。 | [projection.go:375](../../apps/server/internal/dji/projection.go) | [control_command.go:233](../../apps/server/internal/flighthub/control_command.go) |
| `device_network_profiles` | 设备网络、TLS 及通信策略配置。 | [device_network.sql:8](../../apps/server/internal/database/queries/device_network.sql) `RecordNetworkValidation` → [device_network.go:76](../../apps/server/internal/httpapi/device_network.go) | [command_dispatch.go:84](../../apps/server/internal/dji/command_dispatch.go) |
| `device_protocol_cursors` | DJI 有序主题门禁：UPSERT 的时间比较和 RETURNING 决定是否接受乱序消息，实际消费已有状态。 | [ingress.go:74](../../apps/server/internal/dji/ingress.go) | —（见用途说明） |
| `device_protocol_messages` | DJI 协议去重 inbox：唯一键 + INSERT ON CONFLICT RETURNING 决定重复消息，不需要另写 SELECT。 | [ingress.go:53](../../apps/server/internal/dji/ingress.go) | —（见用途说明） |
| `device_relationships` | Dock、Drone 等设备拓扑和父子关系，指令路由实际读取。 | [sync.go:370](../../apps/server/internal/connector/sync.go) | [sync.go:370](../../apps/server/internal/connector/sync.go) |
| `device_stream_channels` | 设备摄像头通道与稳定标识，直播页面和启动策略读取。 | [projection.go:260](../../apps/server/internal/dji/projection.go) | [flighthub_realtime_operations.go:245](../../apps/server/internal/httpapi/flighthub_realtime_operations.go) |
| `device_telemetry` | 历史遥测分区父表，直播与时空轨迹查询使用。 | [projection.go:366](../../apps/server/internal/dji/projection.go) | [streams.sql:20](../../apps/server/internal/database/queries/streams.sql) `ReadChannelTelemetry` → [streams.go:208](../../apps/server/internal/httpapi/streams.go) |
| `device_telemetry_default` | device_telemetry 的默认物理分区；父表 INSERT 自动路由，不能按无显式 SQL 访问判断闲置。 | —（见用途说明） | —（见用途说明） |
| `device_types` | 设备类型种子目录；驱动、拓扑、能力校验活跃读取，没有用户写接口是预期行为。 | —（见用途说明） | [sync.go:158](../../apps/server/internal/connector/sync.go) |
| `devices` | 内部设备目录与所属项目。 | [sync.go:328](../../apps/server/internal/connector/sync.go) | [command_dispatch.go:84](../../apps/server/internal/dji/command_dispatch.go) |
| `driver_definitions` | 驱动静态目录，由迁移种子维护；设备类型/命令处理实际引用。 | —（见用途说明） | [command_dispatch.go:84](../../apps/server/internal/dji/command_dispatch.go) |
| `event_rule_versions` | 已发布规则版本；识别结果投影按固定版本聚合。 | [airsense_projector.go:50](../../apps/server/internal/flighthub/airsense_projector.go) | [legacy_events.sql:14](../../apps/server/internal/database/queries/legacy_events.sql) `GetLegacyPerceptionEvent` → [legacy_events.go:25](../../apps/server/internal/httpapi/legacy_events.go) |
| `event_rules` | 算法事件规则及当前版本配置。 | [airsense_projector.go:36](../../apps/server/internal/flighthub/airsense_projector.go) | [airsense_projector.go:34](../../apps/server/internal/flighthub/airsense_projector.go) |
| `generated_report_evidence` | 正式报告版本的一对多证据清单、校验和及链接，报告聚合和生成均实际使用。 | [task_step.go:249](../../apps/server/internal/report/task_step.go) | [report_read.go:28](../../apps/server/internal/httpapi/report_read.go) |
| `generated_report_versions` | 正式报告不可变版本及发布状态，区别于草稿。 | [task_step.go:112](../../apps/server/internal/report/task_step.go) | [report_read.go:28](../../apps/server/internal/httpapi/report_read.go) |
| `generated_reports` | 正式报告实体与最新版本指针。 | [task_step.go:106](../../apps/server/internal/report/task_step.go) | [report_read.go:28](../../apps/server/internal/httpapi/report_read.go) |
| `idempotency_records` | 事务写入的请求幂等结果与输入哈希。 | [writes.sql:24](../../apps/server/internal/database/queries/writes.sql) `ReserveIdempotency` → [writes.go:171](../../apps/server/internal/database/writes.go) | [writes.sql:29](../../apps/server/internal/database/queries/writes.sql) `ReadIdempotency` → [writes.go:173](../../apps/server/internal/database/writes.go) |
| `inspection_assessment_revisions` | 每次研判/人工复核的完整修订，不能覆盖掉审核历史。 | [assessment_worker.go:214](../../apps/server/internal/agent/assessment_worker.go) | [inspection_assessment.go:52](../../apps/server/internal/httpapi/inspection_assessment.go) |
| `inspection_assessments` | 当前研判状态及修订指针，智能体 worker 和人工复核共用。 | [assessment_step.go:83](../../apps/server/internal/agent/assessment_step.go) | [assessment_step.go:66](../../apps/server/internal/agent/assessment_step.go) |
| `inspection_evidence_sets` | 冻结的巡检识别证据、模型版本及哈希，供研判和审核校验。 | [detect.go:150](../../apps/server/internal/inspection/detect.go) | [assessment_step.go:44](../../apps/server/internal/agent/assessment_step.go) |
| `inspection_flight_ownership` | 飞行是否被任务管理，原子仲裁手动导入/自动任务对告警的归属。 | [inspection_flight_binding.go:28](../../apps/server/internal/flighthub/inspection_flight_binding.go) | [flight_alert_projector.go:515](../../apps/server/internal/flighthub/flight_alert_projector.go) |
| `inspection_observation_assets` | 冻结巡检观测与原素材的多对多关联及素材版本，算法执行据此验证来源；不是单一父记录可替代关系。 | [observe.go:205](../../apps/server/internal/inspection/observe.go) | [inspection_trigger.go:19](../../apps/server/internal/algorithm/inspection_trigger.go) |
| `inspection_observations` | 冻结观测清单、时段、来源模式和完整度，与连续设备遥测 observations 不同。 | [observe.go:200](../../apps/server/internal/inspection/observe.go) | [assessment_step.go:44](../../apps/server/internal/agent/assessment_step.go) |
| `issue_assignees` | 案件多名处理人及实际派单读取。 | [issue_writes.sql:17](../../apps/server/internal/database/queries/issue_writes.sql) `AddIssueAssignee` → [issue_writes.go:290](../../apps/server/internal/httpapi/issue_writes.go) | [issue_reads.sql:76](../../apps/server/internal/database/queries/issue_reads.sql) `ReadIssueAssignees` → [issue_reads.go:63](../../apps/server/internal/httpapi/issue_reads.go) |
| `issue_events` | 案件处理时间线和状态历史。 | [processor.go:378](../../apps/server/internal/agent/processor.go) | [task_step.go:170](../../apps/server/internal/report/task_step.go) |
| `issue_feedback` | 当前案件反馈、纠错、处置、证据快照及幂等；区别于已退役旧 event_feedback。 | [issue_feedback.sql:23](../../apps/server/internal/database/queries/issue_feedback.sql) `FeedbackInsert` → [issue_feedback.go:104](../../apps/server/internal/httpapi/issue_feedback.go) | [issue_feedback.sql:1](../../apps/server/internal/database/queries/issue_feedback.sql) `FeedbackReplay` → [issue_feedback.go:76](../../apps/server/internal/httpapi/issue_feedback.go) |
| `issue_links` | 案件到素材/识别/任务/研判的多类型关联，并统一保存巡检 source_key。 | [airsense_projector.go:143](../../apps/server/internal/flighthub/airsense_projector.go) | [processor.go:171](../../apps/server/internal/agent/processor.go) |
| `issues` | 案件实体与状态、优先级。 | [airsense_projector.go:130](../../apps/server/internal/flighthub/airsense_projector.go) | [processor.go:166](../../apps/server/internal/agent/processor.go) |
| `live_streams` | 直播生命周期、播放引用和设备关联。 | [command_dispatch.go:330](../../apps/server/internal/dji/command_dispatch.go) | [command_dispatch.go:84](../../apps/server/internal/dji/command_dispatch.go) |
| `observations` | 设备观测来源及有类型的位置、姿态/速度/精度；一条位置上报一次 INSERT，位置原设备/时间独立保留。 | [ingestor.go:203](../../apps/server/internal/telemetry/ingestor.go) | [flight_projector.go:764](../../apps/server/internal/flighthub/flight_projector.go) |
| `outbox_consumptions` | 各消费者去重和处理完成状态，一条事件可以有多消费者。 | [outbox.go:171](../../apps/server/internal/outbox/outbox.go) | [outbox.go:81](../../apps/server/internal/outbox/outbox.go) |
| `outbox_events` | 可靠异步事件、领取锁、重试和未知事件保留。 | [processor.go:367](../../apps/server/internal/agent/processor.go) | [lease.go:154](../../apps/server/internal/connector/lease.go) |
| `perception_events` | 结构化感知事件、时间区间及聚合组；旧反馈历史现在存于父记录快照。 | [airsense_projector.go:112](../../apps/server/internal/flighthub/airsense_projector.go) | [airsense_projector.go:82](../../apps/server/internal/flighthub/airsense_projector.go) |
| `project_events` | 项目业务时间线及 SSE 游标，不承担后台重试，不能与 outbox 直接等同。 | [processor.go:382](../../apps/server/internal/agent/processor.go) | [flight_projector.go:543](../../apps/server/internal/flighthub/flight_projector.go) |
| `project_feature_flags` | 项目功能配置、控制准入及验收标记，配置接口和控制链实际使用。 | [project_features.sql:10](../../apps/server/internal/database/queries/project_features.sql) `EnsureProjectFeatures` → [project_features.go:166](../../apps/server/internal/httpapi/project_features.go) | [control_command.go:233](../../apps/server/internal/flighthub/control_command.go) |
| `project_permissions` | 项目显式权限读取用于实时授权与执行前复验；当前未提供管理写接口，保留权限覆盖语义。 | —（见用途说明） | [revalidate.go:17](../../apps/server/internal/agent/revalidate.go) |
| `projects` | 团队下项目实体及配置。 | [directory.sql:73](../../apps/server/internal/database/queries/directory.sql) `CreateProject` → [directory.go:202](../../apps/server/internal/httpapi/directory.go) | [revalidate.go:17](../../apps/server/internal/agent/revalidate.go) |
| `sessions` | 认证会话、撤销和过期处理。 | [http_sessions.sql:4](../../apps/server/internal/database/queries/http_sessions.sql) `CommitHTTPSession` → [session_store.go:39](../../apps/server/internal/httpapi/session_store.go) | [http_sessions.sql:1](../../apps/server/internal/database/queries/http_sessions.sql) `FindHTTPSession` → [session_store.go:27](../../apps/server/internal/httpapi/session_store.go) |
| `task_run_steps` | 每次运行步骤的状态、输入/输出及恢复检查点。 | [assessment_worker.go:172](../../apps/server/internal/agent/assessment_worker.go) | [assessment_worker.go:34](../../apps/server/internal/agent/assessment_worker.go) |
| `task_runs` | 任务实例与控制状态，区别于版本定义。 | [assessment_worker.go:241](../../apps/server/internal/agent/assessment_worker.go) | [assessment_worker.go:34](../../apps/server/internal/agent/assessment_worker.go) |
| `task_steps` | 发布版本的步骤定义及依赖。 | [task_drafts.sql:31](../../apps/server/internal/database/queries/task_drafts.sql) `TaskDraftCopySteps` → [task_drafts.go:260](../../apps/server/internal/httpapi/task_drafts.go) | [assessment_worker.go:52](../../apps/server/internal/agent/assessment_worker.go) |
| `task_trigger_records` | 任务调度触发去重、领取与恢复；不是任务运行副本。 | [schedule_v2.go:74](../../apps/server/internal/tasktrigger/schedule_v2.go) | [schedule_v2.go:162](../../apps/server/internal/tasktrigger/schedule_v2.go) |
| `task_versions` | 不可变任务定义版本，历史运行固定引用。 | [flight_projector.go:190](../../apps/server/internal/flighthub/flight_projector.go) | [assessment_worker.go:34](../../apps/server/internal/agent/assessment_worker.go) |
| `tasks` | 任务实体、配置与当前版本指针。 | [flight_projector.go:173](../../apps/server/internal/flighthub/flight_projector.go) | [task_step.go:33](../../apps/server/internal/agent/task_step.go) |
| `team_members` | 当前成员身份和角色；授权读取实时生效。 | [directory.sql:67](../../apps/server/internal/database/queries/directory.sql) `CreateTeamOwner` → [directory.go:167](../../apps/server/internal/httpapi/directory.go) | [revalidate.go:17](../../apps/server/internal/agent/revalidate.go) |
| `teams` | 团队实体。 | [directory.sql:64](../../apps/server/internal/database/queries/directory.sql) `CreateTeam` → [directory.go:163](../../apps/server/internal/httpapi/directory.go) | [directory.sql:1](../../apps/server/internal/database/queries/directory.sql) `ListTeams` → [directory.go:44](../../apps/server/internal/httpapi/directory.go) |
| `telemetry_event_dedup` | 跨适配器遥测去重门禁：唯一键/INSERT RETURNING 决定是否进入历史和当前投影。 | [projection.go:354](../../apps/server/internal/dji/projection.go) | —（见用途说明） |
| `users` | 账号、认证资料及历史操作者身份。 | [auth.sql:12](../../apps/server/internal/database/queries/auth.sql) `CreateDefaultAdmin` → [database.go:53](../../apps/server/internal/database/database.go) | [flighthub_realtime_operations.go:307](../../apps/server/internal/httpapi/flighthub_realtime_operations.go) |

## 0090 已处理的原表

以下 25 个旧物理表全部有去向；八张作业表改成视图并新增一个基表，净减少 24 张。

| 原表 | 原用途与处理 |
|---|---|
| `connector_action_jobs` | 飞行动作作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `connector_live_action_jobs` | 直播动作作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `connector_geospatial_action_jobs` | 地理资源写入作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `connector_model_jobs` | 模型导入作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `connector_model_delete_jobs` | 模型删除作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `connector_object_upload_jobs` | 航线对象上传作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `connector_device_admin_jobs` | 设备管理作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `connector_management_write_jobs` | 成员/组织管理作业为真实业务；迁到 connector_jobs，并以原名过滤视图保留读取/更新合同。 |
| `detection_group_members` | 检测聚合成员迁到 detections.group_id/grouped_at；保留删除组后识别目标。 |
| `inspection_connector_policies` | 任务管理告警开关迁到 device_adapters.task_managed_alerts。 |
| `inspection_alert_sources` | 私有外部告警证据和飞行来源迁到 connector_remote_resources 私有列。 |
| `inspection_flight_bindings` | 业务 Run/Step 与物理飞行 Run 绑定迁到 connector_jobs，已绑定身份不可变。 |
| `inspection_issue_sources` | 巡检 source_key/assessment 迁到 issue_links，保留项目内来源去重。 |
| `agent_draft_evidence` | 草稿证据引用迁到 agent_drafts.evidence_refs_json。 |
| `platform_audit_events` | 平台审计合并 audit_events.scope，原平台 ID 单独保存以防与项目 ID 冲突。 |
| `retention_policies` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。 |
| `retention_cleanup_runs` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。 |
| `retention_holds` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。归档中的有效保留令迁到 assets.legal_hold/retention_reason，原已有标志继续保留。 |
| `retention_deletion_tombstones` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。 |
| `alert_automation_policies` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。 |
| `alert_automation_policy_versions` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。 |
| `alert_automation_runs` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。 |
| `alert_automation_drafts` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。 |
| `safety_policy_versions` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。历史快照 ID 保留而去除无用 FK。 |
| `sensor_calibrations` | 模块未接入当前生产读写/运行调度；原记录归档 audit_events。历史快照 ID 保留而去除无用 FK。 |

## 保留的边界与验证

project_permissions 和 device_capability_grants 当前没有管理写接口，但授权查询真实读取其数据，删除会改变用户权限。connector_sync_runs 无当前展示接口，但设备外部身份的 last_sync_run_id 外键保存同步来源，且同步调度器持续写入，不能归类为预留空表。

outbox 与 project_events 分别承载可靠后台投递和用户时间线/SSE；观测与位置共用 observations，旧 poses 是过滤视图；巡检冻结清单 inspection_observations、素材清单 inspection_observation_assets 分别承担不同时间和来源合同。审批票据、版本历史、巡检复核修订及正式报告证据也是实际一对多结构，不宜为了数量合成一个无约束 JSON 表。

Qdrant 未新增表；素材及视频完整结果仍由 PostgreSQL 摘要 + 对象存储保存。

本地验证记录见 [精简迁移说明](compact-platform-schema.md)。代码审计证明已注册/被调用的路径及数据合同，不代表已对每个外部设备和所有云服务做线上验收。生产库尚未执行此次清理。
