# 数据库精简迁移

变更：`0090_compact_platform_schema.sql` + `0091_finish_schema_usage_cleanup.sql` + `0092_inline_active_extensions.sql`。业务物理表从 115 张降至 91 张，再经逐表复核降至 87 张，合并三组活跃一对一扩展后为 **84 张**（均包含遥测默认分区；不包含迁移账本和 PostGIS 扩展表）。历史迁移文件字节保持不变。全部表的实际调用与保留/移除依据见 [全量用途审计](schema-table-usage-audit.md)。

## 存储去向

| 原结构 | 新结构 |
| --- | --- |
| 八张 connector 作业表 | `connector_jobs`，以 `job_type` 区分；原名称为带 CHECK OPTION 的过滤视图 |
| `detection_group_members` | `detections.group_id` / `grouped_at` |
| `inspection_connector_policies` | `device_adapters.task_managed_alerts` |
| `inspection_alert_sources` | `connector_remote_resources.inspection_flight_id` / `inspection_evidence_json` 私有列 |
| `inspection_flight_bindings` | `connector_jobs.business_run_id` / `business_step_id` / `business_bound_at` |
| `inspection_issue_sources` | `issue_links.source_key` / `assessment_id`，角色 `inspection_source` |
| `agent_draft_evidence` | `agent_drafts.evidence_refs_json` |
| `platform_audit_events` | `audit_events.scope='platform'`，旧 ID 在 `legacy_platform_id` |
| `asset_upload_intents` | 原文件名在 `assets.metadata_json.uploadFileName`；全部旧意向归档 |
| `evidence_links` | 完整旧关联归档；已发布素材的敏感下载和保护标志迁到 assets |
| `event_feedback` | `perception_events.legacy_feedback_json` 只读历史；新反馈仍由 issue_feedback 存储 |
| `asset_derivatives` | `assets.derivative_source_asset_id` / `derivative_type`，生成参数在 metadata；旧多来源全量归档 |
| `device_command_protocol_correlations` | `device_commands.protocol_*`；独立协议状态与原关联 ID 保留 |
| `connector_asset_access_refs` | `assets.remote_*` 私有列；资产 ID 与加密 AAD 不变 |
| `poses` | `observations.pose_*` 有类型的位置/姿态/速度/精度；旧设备及时间独立保留 |

八类作业的过滤视图不存储数据，继续供类型专属读取和更新使用；带 ON CONFLICT 的新增写入统一访问基表，幂等键包含 job_type。旧表的字段必填、状态、目标和重试约束以类型条件保留，项目/对象组合外键继续存在。开放模型上传的加密凭据会话仍由 `connector_open_model_uploads` 独立保存。

业务工作流 Run 与实际飞行 Run 不合并：task_run_id 表示物理飞行，business_run_id 表示业务工作流，业务 Step 外键及独占约束保留；已绑定的身份不可变更。任务完成继续由原有工作流驱动。

一个案件的一次研判可以包含多个 source_key；巡检来源行按项目/source_key 唯一，普通关联按 issue/link_type/target 去重的索引限定 source_key IS NULL，对应 upsert 带相同谓词。迁移验证同一案件/研判的多来源不会丢失或冲突。

平台审计的 project_id/team_id 为空，项目审计必须同时有 project_id/team_id；项目读取继续限定 project_id。私有告警证据不会放入 summary_json。

## 0092 活跃扩展合并

这三张原表都真实参与生产读写，只合并其一对一存储。旧名称保留过滤视图，合计 12 个兼容视图；无数据副本。对应的完整性 CHECK、项目组合外键、协议 transaction/business 唯一键、私有访问摘要唯一键，以及位置设备/时间、项目/时间和 GiST 空间索引保留。协议关联序列迁到父表，历史关联 ID 和后续分配继续有效。

观测原位置、标准位置与历史 pose 的位置可以不同；pose 的设备/拍摄时间也可不同，迁移逐列保留，不强行覆盖。新位置遥测由 ingestor 一次 INSERT observations，通用原始/标准几何和 typed pose 字段一起写入，未知 CRS 保持标准位置为空、空间质量 unusable。

兼容视图通过触发器写入父表扩展字段；删除视图行只清空扩展。原来源适配器、远程资源或位置设备被删时，组合 FK 的 SET NULL 配合清理触发器移除整个扩展，不误删父业务行。核心新增/UPSERT 已改为直接写父表，不依赖视图 ON CONFLICT。修改扩展身份被拒绝，正常回执更新仍保持业务状态与协议状态各自独立。

素材私有远程凭据不进入 metadata_json，公开素材、快照和回放继续显式选列；密文原样移动，解密 AAD 仍使用原资产 ID 与项目。历史访问引用创建/更新时间与素材自身时间分别保留。

素材 captured_at 是无时区 UTC 时间；公开回放测试发现默认窗口或显式 +08:00 窗口绑定为本地时间时会漏读素材，ReplayMedia 的参数现统一转 UTC。测试同时覆盖默认窗口和显式非 UTC 窗口。

## 退役模块与历史数据

删除四张 retention 表、四张 alert_automation 表、safety_policy_versions、sensor_calibrations。原有记录按原始完整 JSON 归档到现有 audit_events：action=`schema.archive`、resource_type 为原表名、resource_id 为原 ID、details_json 为原记录、actor_system=`schema-migration`。原安全策略和校准引用字段作为历史 ID 快照保留，退役记录从归档回查。

历史操作者外键改为引用 users，不再引用 team_members；当前权限仍由原来的成员、权限查询检查。活跃授权如 project_permissions、device_capability_grants 仍随离队失效。

0091 的四张表同样按原始完整行归档，request_id=`schema-0091`。文件名按最新 completed_at/created_at/id 选择，优先于展示名称但不覆盖原 name。已发布旧证据的素材不允许删除、修改项目身份或去除 legacyPublishedEvidence 标志，敏感下载继续检查与审计。旧事件 API 保留原反馈 ID、值、原因及时间顺序；用户名优先读取当前 users，旧姓名作为历史兜底。派生来源使用项目组合外键；历史多来源选择最早主来源，其余完整边可从审计归档恢复。

0090 归档的有效 retention_holds 在 0091 映射为 assets.legal_hold；已有保留原因优先，空原因使用原令的 reason。完整旧令仍在审计归档。

asset.available 的缩略图消费者已经注册，但当前导入没有发布该事件；保留兼容消费者并将保存关系合并到素材，不宣称当前导入已自动生成缩略图。去重门禁、权限读取、同步来源外键、种子目录和自动分区都经单独检查保留。

## 视频结果

普通视频分析帧不创建 assets，也不为逐帧 HTTP 调用新增 algorithm_run_attempts。对象存储保留：

- `projects/<project>/algorithm-runs/<run>/frames/<index>.jpg`：抽帧图片。
- 同目录 `.json` / `.raw.json` / `.attempts.json`：标注、原始结果、调用诊断。
- `annotations.jsonl`：完整时间轴。

运行级摘要仍在 algorithm_runs，最多每秒更新一次。S3/OSS 使用短期签名直读；本地对象存储由 `/algorithm-assets/frames/<run>/<index>` 提供带项目、运行、帧序号、校验和及有效期签名的访问。算法输入的 AssetID 指向原视频，context.frameIndex/mediaTimeSeconds 明确帧来源。已有素材和证据引用不会删除；若后续将某帧正式引用为证据，应从该帧对象创建正式素材。

## 上线方式

发布时使用配套的新应用与新增迁移，按仓库现有 Go migrate 流程执行；线上生效版本以部署提交和迁移账本为准。上线前保留正常数据库备份。迁移会取得表锁，应选择低峰时间；迁移内的搬迁与删表是同一事务，失败回滚。旧版本应用不可直接在精简后的数据库上运行（旧 INSERT ON CONFLICT 指向视图会失败）。

## 本地验证

使用独立 Docker PostGIS 17 / 3.5，显式 AEROSIGHT_MIGRATION_TEST_DATABASE_URL 指向本机 55439 端口；测试自动创建及删除随机测试库，不连接生产。

已通过：新库迁移与 schema 快照一致；带旧数据升级、八类作业 UUID/检查点保留、飞行绑定、私有来源、草稿证据、审计 ID 冲突处理、历史模块归档、成员离队；巡检证据和案件去重；视频抽帧、对象存储诊断、数据库素材数量不增长、完整 JSONL 播放、重启续跑与跨项目隔离；TypeScript、前端测试、sqlc 校验与 pnpm build。

完整回归另外发现原始 HEAD 上也失败的两个已有测试：TestDockLiveFirstStartWithoutAcceptance（期望 409，当前返回 200）和 TestReportAggregateIncludesAllEvidence（旧查询参数路径断言与现有路径不同）。控制心跳测试曾在完整回归中失败，单独复测及原始 HEAD 复测通过，属于时序敏感项。最终 `pnpm check` 已运行完毕：上述两个原有断言失败，其余 Go 包、前端测试和类型检查通过；本次新增的迁移与视频测试通过。`pnpm build`、最终服务端构建和 `pnpm db:check` 通过。

0091 补漏验证已通过：独立新库及当前 schema 精确一致；含历史上传意向、已发布证据、旧反馈及多来源派生关系的 0090→0091 升级；未绑定 pending 意向完整归档；原展示名称和下载文件名分别保留；原反馈值及时间顺序、用户名更新和成员离队；同一案件/研判多个 source_key；有效保留令；敏感素材标志及删除保护；派生重复执行无重复素材且跨项目来源被拒绝。相关 migrations/issue/flighthub/perception/media 包和媒体 HTTP 测试通过。

`pnpm test:migrations` 通过，维护夹具在 0089 验证旧约束后升级到当前结构，并验证全量新建、既有库和重复执行。原平台共享算法服务模型下，跨项目使用同一个管理员服务是有效配置，原维护夹具的拒绝断言已同步；算法定义/版本/运行的项目隔离断言保留。

`pnpm drill:upgrade-rollback` 默认模式通过旧页面 SQL 数据合同、证据素材/保留标志及未知事件不被消费的验证；默认没有启动旧应用进程，不表示任意旧应用可写入新结构。完整 `pnpm build`、sqlc 校验及 OpenSpec 严格验证通过。检查日志在 `.build/usage-cleanup-*`，均连接本机测试数据库。

0091 最终全量 `pnpm check` 已完成（`.build/usage-cleanup-final-check.log`）：仍仅上述两个原始 HEAD 的旧断言失败，其余 Go 包、当前媒体/巡检路径、新增升级测试、前端与类型检查通过。最终服务端重新构建并嵌入 92 个当前迁移文件；原有已发布迁移无修改，构建生成的前端声明路径噪声已恢复。

0092 验证通过：新库及 schema 精确一致；含三个完整旧扩展的升级前/后 JSONB 逐字段一致；原密文按原资产 ID 解密；协议序列继续分配、事务及业务幂等；私有引用去重；跨项目来源、非法精度及扩展身份变更拒绝；视图删除、来源适配器/资源/独立位置设备删除均保留业务父记录。位置入库测试额外施加 INSERT 时必须已有 pose 的 CHECK，验证 WGS84 与未知 CRS 都由同一 INSERT 写完整字段，重放不重复。

实际 SQL 数据库链路也通过：DJI 发布、ACK、NACK、超时、迟到回执及断开路由；司空遥测去重/次序、模型素材投影；HTTP 远程素材消费、轨迹、时空工具、媒体访问；公开素材/快照/回放实际包含素材而不带私有摘要/密文。最终相关包回归在 `.build/one-to-one-final-contracts.log`，最后的直接协议写入复测在 `.build/one-to-one-dji-final.log`；额外约束/删除用例已再次通过。

`pnpm check` 全量运行记录为 `.build/one-to-one-final-check.log`：原有两个失败仍在；本轮新测试另外发现的素材回放时区问题已修复，之后相关 HTTP 测试全部复测通过（`.build/one-to-one-privacy.log` / `one-to-one-final-contracts.log`）。没有声称全量 check 全绿。`pnpm build`、最终服务端重新构建、sqlc、维护迁移夹具、升级回滚 SQL 合同演练均通过；服务端嵌入 93 个迁移文件。

## 2026-10-09 发布前真实数据演练

已从线上 PostgreSQL 16.4 生成完整 custom-format 备份，取回后 SHA-256 与服务器文件一致；备份和包含数据的演练库均不提交到仓库。恢复后使用候选程序应用 0090–0092，保留 27 个素材、152346 条观测/位置和两个远程访问引用；三个扩展的升级前后全行 JSONB 指纹一致，父记录数量一致。迁移账本为 93 条。

线上原有 117 张业务物理表，包含两个动态月度遥测分区；演练后为 86 张，即 84 张基础业务物理表（包含默认分区）加两个动态分区。动态分区按遥测月份继续保留。

恢复演练同时发现 Windows 的自动 CRLF 转换会改变本地嵌入的旧迁移校验值。`.gitattributes` 现固定迁移文件为 LF；已按 Git 原始字节还原本地文件，90 条既有迁移与线上账本全部一致，未修改其仓库内容。候选程序重新构建，升级演练及迁移/快照测试通过。演练证据在 `.build/backups/aerosight-compact-rehearsal-20261009.json`。
