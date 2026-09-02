## 1. 官方契约与数据基础

- [x] 1.1 将官方 released 目录的 89 个接口写入版本化 endpoint manifest，包含 method、path、scope、risk、分页、部署限制和验证级别，并用清单测试断言总数为 89、方法统计为 59 GET/19 POST/6 PUT/5 DELETE。
- [x] 1.2 为系统、组织、设备、控制、任务、直播、地图和模型领域补齐脱敏成功/空状态/业务失败 fixture，并用扫描测试证明 fixture 不含 Token、完整 SN/UUID、真实坐标、签名 URL 或其他秘密。
- [x] 1.3 为 `connector_resource_sync_states`、`connector_remote_resources` 和 `connector_capability_snapshots` 增加向前数据库迁移、枚举约束、项目复合外键和幂等唯一键，并通过数据库 schema 测试及迁移重放测试。
- [x] 1.4 实现上述投影的项目级 repository 与事务 upsert/link/missing 操作，并用跨项目 ID/远端 UUID 冲突测试验证租户隔离和 canonical link 约束。
- [x] 1.5 注册司空各领域 read/action capability、风险等级和默认关闭的 feature flags，并用 manifest 交叉测试证明运行时不能声明 endpoint manifest 或 Driver 未定义的能力。

## 2. 安全 OpenAPI 客户端基础

- [x] 2.1 将共享 transport 扩展为受约束的 GET/POST/PUT/DELETE JSON 请求，保留 request ID、超时、响应大小和 HTTP/business 双层校验，并用每种方法及畸形响应单元测试验证。
- [x] 2.2 实现 path template、query/body 编码和固定官方 origin 校验，禁止绝对 URL 与自动重定向，并用 SSRF、路径逃逸和跨主机重定向测试验证 Token 不会外发。
- [x] 2.3 建立 endpoint 级业务码注册表，至少正确归一化成功、合法空状态（含 `231011`）、参数/上下文错误（含 `200610`）、权限、认证、限流、暂时故障与 schema 不兼容，并通过 fixture 表驱动测试。
- [x] 2.4 实现共享 token bucket、最大并发、`Retry-After`、有界抖动退避、分页上限和完整快照判定，并用 fake clock 测试 429、5xx、分页中断、重复页及恰好达到上限的失败关闭。
- [x] 2.5 实现响应内上传/下载/直播/模型链接的用途、协议、主机和有效期校验，并用允许/拒绝主机及过期 URL 测试验证不会形成 SSRF 或永久凭据。
- [x] 2.6 实现系统健康、STS、SN 解密、组织/角色/权限、项目用户/成员/加入码的 typed clients，并用官方脱敏 fixture 覆盖所有请求头、scope 和响应 schema。
- [x] 2.7 为 Token、SN 解密结果、STS、签名 URL、直播 Token 和上游错误增加统一 redactor，并用日志、trace、metrics、审计和 API snapshot 测试证明明文不会出现。

## 3. 设备状态、位置与地图首个交付切片

- [x] 3.1 实现设备详情、物模型 state、HMS、历史拓扑和自动录制配置 typed clients，并用 Dock 2/M3TD、字段缺失、未知型号和非法坐标 fixture 验证解析。
- [x] 3.2 建立按 `device_model.key` 版本化的 Dock/飞行器物模型 mapper，覆盖位置、姿态、heading、高度、mode、网络、电池、环境和直播状态，并用字段矩阵测试确保未知字段只进入诊断且不扩张能力。
- [x] 3.3 在 Connector 租约内实现独立 `device-state` 与 `health` 资源流、在线/离线自适应轮询和游标/退避恢复，并用调度器测试证明慢目录或 HMS 失败不阻塞位置更新。
- [x] 3.4 将有效物模型数据事务写入 `device_telemetry`、`device_latest_telemetry`、`observations` 和 `poses`，保留采集/接收时间、来源、原坐标和转换版本，并用数据库集成测试验证幂等、乱序与无效坐标行为。
- [x] 3.5 将状态新鲜度映射为 online/degraded/offline/unknown，并用轮询失败和时间推进测试证明旧值不会被当前时间伪装为实时事实。
- [x] 3.6 将 HMS 生命周期映射到 `issues`、拓扑历史映射为带有效时间关系、自动录制映射到只读配置投影，并用重复告警和拓扑变更集成测试验证。
- [x] 3.7 修复禁用/断开连接器仍能触发同步、纳管和 active binding 的生命周期门禁，并用 Web API、outbox 与 Worker 三层测试证明历史可读但新写入被拒绝。
- [x] 3.8 修正地图分类以将 `aircraft`/`drone`/`uav` 统一归入无人机，并用 map model 单元测试及 Dock + M3TD 快照测试验证图层和图标。
- [x] 3.9 在设备详情和项目地图展示位置、状态、数据新鲜度、来源及无效/未校准原因，并用 Web 组件测试验证有位置、无位置和过期三种状态。

## 4. 能力探测与连接器诊断

- [x] 4.1 实现只调用无副作用 GET 的领域 capability probe，计算官方契约、区域/部署、账号/设备响应、实现与验收四层交集，并用公有云不适用、403、空列表和未知业务码测试验证。
- [x] 4.2 持久化带证据级别、型号/固件范围、探测时间和过期时间的 capability snapshot，并用固件变化与过期测试证明高风险能力自动收窄而不扩张。
- [x] 4.3 增加项目授权的连接器诊断 API，返回各资源流水位和 `supported/empty/forbidden/not_applicable/unverified/degraded/failed`，并用成员/管理员及跨租户 API 测试验证脱敏与权限。
- [x] 4.4 在连接器页面实现能力矩阵、验证徽标、同步水位、限流/权限/兼容错误和只读重新探测入口，并用 UI 测试验证空状态不降低整体健康。
- [x] 4.5 增加官方 endpoint manifest/fixture 漂移检查，发现 method/path/schema 变化时将对应能力标为待验证，并用修改后的测试 manifest 验证高风险 action 不会自动开放。

## 5. 航线与飞行任务全链路

- [x] 5.1 实现 wayline 列表/详情/上传完成通知及 flight-task 列表、recent、batch、default-name、detail、单项、dispatch-check、status、resumption typed clients，并用每个 endpoint fixture 验证参数与 schema。
- [x] 5.2 实现航线与任务目录资源流、分页/完整性和 `connector_remote_resources` 幂等投影，并用重复同步、部分分页失败及同远端 UUID 跨项目测试验证。
- [x] 5.3 将可转换航线关联 `tasks/task_versions`，将远端飞行任务关联 `task_runs` 并实现状态机映射和时间线对账，用受理、运行、暂停、成功、失败、取消及未知超时集成测试验证。
- [x] 5.4 实现任务 track 与操作日志同步，将 timestamp/latitude/longitude/height 写入时空轨迹并关联 task run，用乱序/重复轨迹和无效坐标测试验证。
- [x] 5.5 实现任务 media、export history 和单个飞行记录下载引用同步，将产物幂等关联 `assets` 且按需刷新临时 URL，并用 URL 过期、跨项目和重复媒体测试验证。
- [x] 5.6 实现 flight-alerts 与 ai-alert-record 同步，归一化到 `issues`/`perception_events` 并关联任务、设备、空间和媒体，用告警更新/恢复/重复测试验证不重复触发。
- [x] 5.7 实现 STS 获取、受控对象上传、航线上传完成通知的持久上传工作流，并用响应丢失恢复测试证明先查远端再决定是否重试。
- [x] 5.8 将 dispatch-check、任务创建、状态更新和断点续飞接入项目 RBAC、安全预检、审批、幂等作业与远端对账，并用 HTTP 受理但最终未知的集成测试证明不误报成功或盲目重飞。
- [x] 5.9 增加飞行运营页面/API，展示航线、任务运行、轨迹、媒体、飞行记录、告警和写操作最终状态，并用只读成员、操作者和跨租户 E2E 测试验证。

## 6. 直播、录制、分享与码流转发

- [x] 6.1 实现直播启动、画质设置、组织/项目录制任务、自动录制、直播分享和码流转换器全部 typed clients，并用供应商变体、合法空码和短期凭据 fixture 验证。
- [x] 6.2 实现火山、声网、SRS 等官方返回供应商的 adapter 注册与未知供应商失败关闭，并用多供应商响应矩阵测试统一播放描述和秘密剥离。
- [x] 6.3 将官方直播启动、本地播放授权撤销和恢复接入 `live_streams` 状态机与 Worker 证据对账；不得调用未发布的停止接口或因租约过期误报远端停止，处理五分钟无观众、设备离线、凭据到期、响应未知及 Worker 重启，并用 fake clock 集成测试验证有证据时最终收敛、无证据时保持可解释未确认状态。
- [x] 6.4 同步录制、分享和转换器目录，正确映射无资源为空状态，并用连接器健康和幂等投影测试验证列表为空不报故障。
- [x] 6.5 将画质、录制、分享和转换器创建/开关/删除接入独立 capability、RBAC、审计和默认关闭 feature flag，并用未授权与未验收测试证明上游零调用。
- [x] 6.6 实现实时媒体页面/API，展示设备通道、供应商无关播放器、会话状态、录制、分享和转换器，并用短期播放授权、过期和权限撤销 E2E 测试验证。
- [x] 6.7 增加直播秘密与孤儿会话专项测试，扫描数据库/日志/API 响应且模拟 Worker 崩溃，验证 URL/Token 不泄露且会话可恢复或清理。

## 7. 地图标注、飞行区、离线地图与 AirSense

- [x] 7.1 实现项目/workspace 地图 element、flight-area、offline-map 和 air-sense typed clients，并用 GeoJSON、文件 URL、分页和活动/过期告警 fixture 验证。
- [x] 7.2 实现地图标注与飞行区资源流、几何/schema 校验、完整快照和远端版本投影，并用非法 GeoJSON、部分响应与 missing 处理测试证明不会清除上次成功数据。
- [x] 7.3 实现飞行区文件和离线地图短期 URL 的项目授权访问与过期刷新，并用 host allowlist、跨项目和签名参数日志扫描测试验证。
- [x] 7.4 将 AirSense 告警映射到统一安全事件和地图实时图层，并用发生、更新、过期、重复和关联任务测试验证生命周期。
- [x] 7.5 将地图标注创建/更新/删除接入远端版本冲突、RBAC、审计与删除 feature flag，并用并发修改测试证明旧版本不会覆盖新数据。
- [x] 7.6 增加地图空域 UI，展示司空标注、飞行区、离线地图和 AirSense 的来源、版本与新鲜度，并用项目隔离 E2E 测试验证。
- [ ] 7.7 使用已知控制点和一段任务轨迹执行坐标基准现场验收，记录接口级基准/转换版本及误差；验证未通过前保持 `unverified` 且不用于控制预检。

## 8. 模型目录与重建作业

- [x] 8.1 实现 model 列表/详情/下载 URL 与 open_model running/detail/resource/start/stop/delete/token/callback 全部 typed clients，并用完成、运行、失败、空列表和短期凭据 fixture 验证。
- [x] 8.2 实现模型目录与资源投影，将模型/文件关联 `assets` 且按需刷新下载信息，并用幂等、版本更新和跨项目访问测试验证。
- [x] 8.3 将传统模型重建与开放模型 start/stop 作为可恢复长作业接入 outbox、幂等键、进度轮询和最终资产关联，并用创建响应未知、Worker 重启和失败恢复测试验证。
- [x] 8.4 实现开放建模上传 token 与 callback 的短期凭据流程，并用过期、重复 callback、错误资源归属和秘密扫描测试验证。
- [x] 8.5 将模型/资源 DELETE 接入专用 capability、owner/admin、差异预览、审批和默认关闭 feature flag，并用未确认及跨租户测试证明上游零调用。
- [x] 8.6 增加模型页面/API，展示目录、进度、产物、失败原因和实际可用 action，并用无权限、空列表和运行中作业 E2E 测试验证。

## 9. 设备控制、维护与管理域

- [x] 9.1 为 return_home/cancel、flighttask pause/recovery、控制权、云控、相机/镜头、TCA、RTK、relay 和 active-project 建立 action adapter 与输入/输出 schema，并用 endpoint manifest 覆盖测试证明无遗漏。
- [x] 9.2 将离散设备指令接入 `device_commands`/`command_attempts`、状态新鲜度、安全策略、审批、截止时间和幂等规则，并用 HTTP 受理、NACK、超时和物模型对账测试验证。
- [x] 9.3 实现组织/项目 command status 轮询与命令结果关联，处理无法关联、重复和乱序结果，并用 Worker 重启恢复测试证明不提前标记成功。
- [x] 9.4 实现控制权/云控短期独占会话、心跳、频率限制、自动停止和释放，并用双操作者竞争、权限撤销和心跳中断测试验证。
- [x] 9.5 将相机/镜头切换和 TCA 状态接入设备有效 capability 与 UI 前置条件，并用不支持型号、离线设备和状态过期测试验证上游零调用。
- [x] 9.6 将 RTK 标定、中继对频、设备迁移与 SN 解密接入 owner/admin、专用 capability、二次确认/审批、feature flag 和审计，并用默认关闭及失败不自动重试测试验证。
- [x] 9.7 实现组织、用户、角色、权限、项目用户/成员/加入码只读管理视图，并用管理域授权与项目成员权限分离测试验证。
- [x] 9.8 将添加项目成员及未来 released 组织管理写接口接入精确目标预览、管理 capability、审批和结果回读；用普通项目 admin、取消确认和跨组织测试证明未授权时上游零调用。
- [x] 9.9 增加受控操作 UI，按 capability intersection 展示风险、前置条件、审批、feature flag 和最终结果，且用旧客户端直接调用 API 的 E2E 测试证明服务端仍会拒绝绕过。
- [x] 9.10 为每个高风险 action 建立区域/账号/型号/固件现场验收记录和运行时门禁，并用固件版本变化测试验证既有验收不会错误继承。

## 10. 真实验收、回填与发布门槛

- [x] 10.1 提供显式 opt-in 的真实只读 smoke 命令，覆盖所有无副作用 released GET，输出仅含 endpoint、类别、计数/字段集合和耗时；用输出扫描证明不含 Token、SN、UUID、坐标或 URL。
- [ ] 10.2 使用当前本地司空连接对 system/project/device/state/HMS/topology/task/wayline/flight-area/model/open-model/live-share/cloud-control 运行真实只读验收，并将成功、空状态、参数要求和权限差异写入 capability evidence，不修改任何远端资源。
- [ ] 10.3 对项目 1123 启用状态资源流并回填 Dock 2 与 M3TD 的 telemetry/pose，验证数据库各有最新位置、地图同时显示正确设施/无人机图层且刷新时间随真实轮询推进。
- [ ] 10.4 比较司空与本地的航线、任务、模型及其他目录计数和抽样关联，验证本地已同步当前已知的 13 条航线、2 个模型及可访问任务且不泄露原始标识。
- [ ] 10.5 执行跨租户、断开连接器、部分分页失败、429、业务空码、临时 URL、命令未知结果和 Worker 重启的端到端故障矩阵，并保存通过证据。
- [ ] 10.6 在用户批准的测试窗口内逐 action 执行直播、任务/航线上传、建模和低风险写入现场验收，记录前置、远端结果、回滚和审计；未验证 action 保持 feature flag 关闭。
- [ ] 10.7 在用户单独批准且有现场观察员的窗口内逐项验收返航/暂停恢复、控制权/云控、相机镜头、TCA、RTK、对频、迁移、DELETE 和组织权限变更；任何未执行或结果未知项必须保持生产不可用。
- [ ] 10.8 更新司空合同 README、能力覆盖表、部署/限流/故障排查、秘密轮换、回滚和现场验收 runbook，并用文档链接检查及 endpoint manifest 覆盖脚本验证无孤立接口。
- [ ] 10.9 运行全部 Go 测试、数据库迁移/回滚兼容测试、`pnpm check` 和 `pnpm build`，修复所有回归并记录最终命令输出摘要。
