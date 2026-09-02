## Context

见 [proposal.md](./proposal.md) 的动机。当前 `dji.flighthub2@1.0.0` 只实现项目发现和 `GET /openapi/v2.0/project/device` 目录同步；`DirectoryClient` 只能列设备，运行时 manifest 也固定为只读。现有项目 1123 已纳管一套 Dock 2 + Matrice 3TD，但两台设备均没有 `poses`。对当前连接凭据进行固定官方主机、只读 GET 探测后确认：项目、设备详情、历史拓扑、HMS、自动录制、任务、13 条航线、飞行区、2 个模型和开放建模状态均可读取，Dock 与飞行器的 `GET /device/{sn}/state` 均真实返回经纬度；因此地图故障是本地漏接状态接口，不是上游缺数据。

官方 released 目录当前包含 89 个接口（59 GET、19 POST、6 PUT、5 DELETE），覆盖系统/上传凭据、组织权限、设备、控制、直播、航线任务、地图空域和模型重建。接口同时使用 HTTP 状态和业务 `code`；例如合法无分享资源与参数不完整不能统一当作平台故障。公开资料没有稳定公布全局配额，部分设备指令的 HTTP 成功只表示同步受理，真实执行结果需要通过指令状态、任务状态或物模型继续对账。

该变更跨 Next.js、Go Worker、PostgreSQL、媒体网关和外部 DJI API，并包含秘密、长作业和现场物理控制，必须分层实现和分风险启用。

## Goals / Non-Goals

**Goals:**

- 为官方 released 接口建立版本化清单、typed client、能力状态和验证证据，不再只覆盖设备目录。
- 第一交付切片先让已纳管 Dock/飞机获得真实状态、位置和地图展示，再逐步接入只读任务/媒体/空间/模型数据。
- 复用统一设备、遥测、位置、任务、资产、直播、事件、命令、审批和审计模型；厂商投影只保存远端身份与无法由通用模型表达的同步状态。
- 让所有写操作走相同的 RBAC、安全策略、幂等、审批和对账入口，并按风险与现场验收逐项启用。
- 在连接器诊断中精确表达 `supported`、`empty`、`forbidden`、`not_applicable`、`unverified`、`degraded` 和 `failed`。

**Non-Goals:**

- 不将司空公有云连接器替换成 DJI Cloud API MQTT 直连，也不建立自有 DRC Broker。
- 不承诺未在当前账号、型号、固件和区域完成现场验收的高风险操作可生产使用。
- 不缓存或代理大体积媒体、模型、离线地图字节流经过 PostgreSQL/Next.js；只保存受保护引用和必要元数据。
- 不自动执行 DELETE、设备迁移、成员权限修改或任何可能改变现场/组织归属的动作。

## Decisions

### 1. 以官方契约清单驱动覆盖，而不是手写一个“大客户端”

在 `contracts/dji-flighthub/v2` 扩充脱敏的 endpoint manifest：记录方法、path template、作用域（组织/项目/workspace/设备）、风险、分页、业务空码、部署限制、响应 schema 版本和验证级别。Go 客户端分为共享 transport 与领域 client；共享层负责固定 origin、headers、大小限制、重试、business code 和脱敏，领域层负责 typed request/response 与 schema 校验。

选择原因：89 个接口若全部散落为手写请求，权限、错误和秘密规则会快速漂移；manifest 可同时驱动 capability probe、fixture 覆盖率和诊断页。备选方案是导入完整 OpenAPI 自动生成客户端，但官方 Apifox 文档存在动态字段、业务码和部署差异，生成结果仍需要大量手工治理，且容易把未验证写接口直接暴露。

接口清单按以下领域落地：

| 领域 | 主要 API 族 | 本地归属 | 默认启用 |
| --- | --- | --- | --- |
| 系统与凭据 | `health`、`system_status`、项目 STS、SN 解密 | connector health / 受控上传 | 健康读启用；临时凭据按流程启用 |
| 组织与项目 | organizations/users/roles/permissions、project/users/members/join-code | 管理域投影 | 只读探测；写入关闭 |
| 设备与健康 | project/device、device detail/state/HMS、topology history、auto-record | devices / telemetry / poses / issues | 启用 |
| 设备控制 | command、control/status、cloud-controls、camera/lens、TCA、RTK、relay、active-project | device commands | 逐 action 现场启用 |
| 航线与任务 | wayline、flight-task、dispatch-check、resumption、track/media/export/oper/alerts | tasks / task_runs / assets / events | 先读后写 |
| 直播媒体 | start、quality、record streams、live-shares、stream-converters | stream channels / live_streams | 先读；控制分项启用 |
| 地图空域 | elements、flight-areas、offline-maps、AirSense | spatial projections / issues | 读取启用；标注写入受控 |
| 模型 | model、open_model reconstruction/resource/store | jobs / assets / remote projection | 先读后写 |

### 2. 使用一个受约束的 transport，所有上游链接再次过主机策略

扩展当前 `request` 支持 GET/POST/PUT/DELETE、JSON body、query 和 endpoint profile，但调用者不能传任意绝对 URL。API origin 固定为 `https://es-flight-api-cn.djigate.com`（后续区域由部署级 allowlist 选择），禁止自动重定向。响应中的上传、下载、直播或模型链接先解析用途、主机、协议和有效期；只有 endpoint manifest 允许的链接类型才可短期下发或由 Worker 使用。

选择原因：沿用已验证的 token、timeout、response limit、request ID 和 retry 边界，同时防止 SSRF 与凭据跨主机。备选方案是每个领域维护独立 HTTP client，会重复安全逻辑且难以证明所有接口都遵守相同边界。

### 3. 同步按资源流分层调度并复用 Connector 租约

每个连接器仍由单一 Connector 租约所有，但内部建立有界调度器：

- `inventory`：低频完整目录与历史拓扑。
- `device-state`：在线/活动设备高频轮询，静止或离线设备降低频率；初始建议分别为 15 秒与 60 秒，部署可收紧但不能绕过上游限流。
- `health`：HMS、自动录制和连接器能力。
- `active-operations`：执行中任务、直播、控制命令与建模任务的短间隔对账。
- `catalogs`：航线、历史任务、地图、飞行区、离线地图和模型的低频分页同步。

所有流共享连接器级 token bucket、最大并发、有抖动退避和 `Retry-After`。新增 `connector_resource_sync_states` 保存 `(project_id, connector_instance_id, resource_kind)` 的游标、成功水位、退避和最后错误类别，避免一个模型接口失败阻塞设备位置。完整快照只有在所有页和 schema 都通过时才处理 missing。

备选方案是每次目录扫描顺序调用全部接口；这会让一个慢接口拖垮位置时效，也无法独立恢复分页。

### 4. 状态归一化采用 schema-on-read 映射和原始引用，不保存秘密原文

按 DJI `device_model.key` 选择版本化 mapper，把已知物模型字段写入：

- `device_latest_telemetry` 与 `device_telemetry`：数值、单位、质量、上游采集时间。
- `observations` 与 `poses`：EPSG:4326 位置、heading/姿态、高度、来源和坐标转换版本。
- `devices`/状态投影：online、mode、freshness 和安全可操作状态。
- `device_stream_channels`：相机/直播通道与当前可用性。
- `issues`：HMS 和 AirSense 生命周期；`perception_events`：AI 告警。

厂商原始 payload 不整包长期保存；仅保存响应 schema 版本、request correlation、远端资源 ID、未知字段名/类型摘要和必要的非秘密原始值。未知字段不会扩大能力。坐标先保留 vendor 原值和 `coordinate_reference=unverified`，以一个已知控制点及任务轨迹完成现场确认后才写入转换版本；已确认接口统一输出 EPSG:4326。

地图分类函数同时将 `aircraft`、`drone`、`uav` 归入无人机，避免有 pose 后仍显示为地面设备。

### 5. 使用通用远端资源投影连接现有业务模型

新增小型 `connector_remote_resources` 投影，字段包括项目/团队/连接器、`resource_kind`、远端 ID、远端版本/更新时间、状态、非秘密摘要、canonical target type/id、首次/最后发现时间和 missing 时间；唯一键为 `(project_id, connector_instance_id, resource_kind, remote_id)`。它只负责 vendor identity 与同步对账：

- 远端飞行任务链接 `task_runs`，轨迹进入 poses/observations，媒体进入 assets。
- 航线链接 `tasks`/`task_versions`；无法完整转换的厂商航线先保持只读投影。
- 模型和飞行记录产物链接 assets；重建过程使用持久 job/command 状态。
- 地图 element、飞行区和离线地图保留空间元数据/资产引用，并在统一地图查询层投影。
- HMS、AI、AirSense 分别链接 issues/perception_events。

选择原因：为每个司空名词建专用表会产生平行状态机；只塞入通用 JSON 又无法唯一标识和安全对账。受约束的远端资源表提供稳定关联，业务事实仍由现有 canonical 表拥有。

另新增 `connector_capability_snapshots`，保存探测结果、证据级别、区域/部署、账号权限、型号/固件范围和到期时间。所有新表均包含 `project_id`、`team_id`、复合外键与项目级唯一约束。

### 6. 能力状态是四层交集，并区分自动探测与人工验收

有效能力计算为：官方 endpoint manifest ∩ 当前区域/部署探测 ∩ 账号/项目/设备响应 ∩ AeroSight 实现及验收。探测只自动调用无副作用 GET；任何必须写入才能确认的能力保持 `unverified`。最终状态包含原因和证据时间，并收窄现有 Driver/DeviceType capability，绝不扩张它。

连接器诊断展示每个领域的 `supported`、`empty`、`forbidden`、`not_applicable`、`unverified`、`degraded` 或 `failed`，同时显示 fixture、真实只读和现场写入三种验证徽标。`231011` 等官方无资源业务码进入 `empty`；`200610` 参数/上下文问题进入 `unverified` 或 `configuration_required`，而非 Token 故障。

### 7. 写操作统一进入命令/作业账本，HTTP 200 只表示受理

设备 action 使用 `device_commands`/`command_attempts`；任务、上传、建模和管理 action 使用现有 outbox/作业记录并关联 audit。每个写请求包含 endpoint/action code、稳定 idempotency key、actor、审批、安全策略版本、截止时间和远端 request/cmd/task ID。恢复时先读取远端状态，只有 endpoint manifest 声明可安全重试才重发。

控制结果来源按优先级对账：远端 command status、flight task status、物模型状态变化和明确失败响应。`POST /device/{sn}/command` 的返回不能直接完成命令。控制权/云控使用短期独占会话、心跳与自动释放。DELETE、迁移、RTK/对频、组织成员/角色变更要求专用 feature flag、owner/admin + capability grant、二次确认或审批和目标型号现场验收。

备选方案是由页面直接调用司空并显示响应；它绕过审计和安全互锁，且无法处理响应丢失或物理结果未知，因此拒绝。

### 8. 直播由统一会话包装动态供应商，临时秘密不进入普通列

`live_streams` 继续作为 canonical 会话。司空返回的供应商、凭据摘要和过期时间写入非秘密字段；播放 URL/Token 使用与会话绑定的加密短期凭据，普通 `playback_ref` 不保存可直接使用的供应商凭据。供应商 adapter 将火山、声网、SRS 等响应规范化为浏览器支持的播放描述。

当前官方 released 目录只有 `POST /openapi/v2.0/live-stream/start`，没有停止直播或查询直播会话状态的 endpoint，通用设备 command 契约也没有直播停止 action。因此启动调用官方 start；用户停止、权限撤销或未知供应商时立即撤销本地播放授权、销毁短期凭据并进入 `stopping`/`failed`，但不得调用未进入 endpoint manifest 的虚构 stop API，也不得仅因本地租约过期就宣称远端已停止。

`active-operations` 对账器使用新鲜物模型 `live_status`、设备在线状态、凭据到期和官方“五分钟无观众自动停止”规则作为证据，处理设备离线、Worker 重启和启动响应未知。只有证据足以证明推流终止时才进入 `stopped` 并释放占用；证据互相矛盾或不可用时保持 `stopping`、`failed` 或显式 unknown 原因。恢复时先核对最新证据，绝不盲目重发 start。录制、分享、画质和码流转换使用独立 capability/action；缺少分享资源是空状态。

### 9. Web 只通过项目 API 读取投影和提交意图

Next.js 继续拥有同步数据库访问、团队成员授权和 UI；浏览器不持有组织 Token，也不直接调用 DJI。页面分为：

- 连接器：领域能力矩阵、同步水位、限流/权限/兼容诊断、只读重新探测。
- 设备与地图：状态新鲜度、HMS、正确的 Dock/无人机位置和图层。
- 飞行运营：航线、任务运行、轨迹、媒体、飞行记录和 AI 告警。
- 实时媒体：通道、直播、录制、分享和转换器。
- 空间与模型：标注、飞行区、离线地图、AirSense、模型和重建作业。
- 受控操作：仅渲染有效 capability，并展示风险、前置条件、审批和最终对账状态。

所有查询以 `project_id` + team membership 开始，远端 UUID 只作为项目作用域内的二级键。

### 10. 验证采用四级证据，不用生产写操作做自动测试

1. 契约：89 个 released endpoint 全部进入 manifest，保存脱敏 fixture，校验 method/path/header/body/envelope、分页、空码和秘密 redaction。
2. 本地集成：Go `httptest`、数据库集成和 Web 单元/E2E 验证重试、部分分页失败、跨租户、位置映射、地图分类、命令未知结果和恢复。
3. 真实只读：使用本地加密凭据对固定官方主机运行显式 opt-in smoke，输出只包含计数、字段集合、业务码类别和耗时，不输出 Token、SN、UUID、坐标或 URL。
4. 现场写入：逐 action 使用测试项目/设备、审批清单和观察员人工执行，记录前置/结果/回滚；未完成项保持 feature flag 关闭。

实现期间先运行领域级 Go/Next 测试和 schema 检查，最后运行 `pnpm check`、`pnpm build` 及 Go 全量测试。

## Risks / Trade-offs

- [官方文档与实际响应漂移，且型号字段不同] → endpoint manifest 与 model mapper 版本化，未知字段隔离，schema 失败不破坏上次快照。
- [轮询 89 个接口触发限流或让位置延迟] → 按资源流和活动状态调频，共享 token bucket，尊重 `Retry-After`，位置流与低频目录隔离。
- [官方未明确所有空间数据坐标基准] → 保存原值与未验证标志，用已知控制点现场验收后才启用转换和飞行决策用途。
- [HTTP 受理与物理结果分离导致长期未知] → 持久命令/作业账本、多来源对账、截止时间与人工核查，禁止盲目重发。
- [短期 URL/Token 经日志或数据库泄露] → 字段分类、响应 redactor、加密短存、host allowlist 和自动安全测试。
- [通用远端投影弱化领域约束] → `resource_kind` 枚举、JSON schema、canonical link 约束；业务状态仍由 tasks/assets/issues/live_streams 所有。
- [组织管理和设备迁移影响范围大] → 默认关闭、专用授权与审批、差异预览、精确目标确认和现场验证。
- [一次性 UI 范围过大] → 后端契约可全覆盖，但 UI 按位置/只读运营/直播建模/高风险控制四个切片交付，每个切片独立可验收。

## Migration Plan

1. 增加 endpoint manifest、fixture、错误分类和数据库投影表；部署后不启动新同步，验证向前迁移与旧连接器读取兼容。
2. 启用 capability probe、设备 state/HMS/topology 流和地图分类修复；对项目 1123 回填两台设备状态与 pose，验证位置、新鲜度和断开连接器门禁。
3. 启用只读航线/任务/轨迹/媒体/地图/模型同步，按资源流回填并比较司空计数；异常流可单独暂停或清空其新投影后重建。
4. 启用直播读取和经现场验证的会话控制，再启用航线上传、任务与建模写入。
5. 每个高风险 action 完成独立现场验收后才打开对应 feature flag；不做整组“一键开放”。

回滚时先关闭写 feature flags 和新资源流，等待在途命令完成/进入人工核查，再回滚 Worker/Web。新表与新增列保留以支持旧版本读取和后续重放，不在应用回滚中删除；短期凭据立即过期/撤销。位置回填为追加历史，不回删用户原有 pose。

## Open Questions

- 司空各地图、轨迹和模型定位接口在当前中国公有云账号下的实际坐标基准仍需用已知控制点现场确认；该结果只决定 mapper 的转换参数，不改变上述安全契约或任务拆分。
- 当前账号对 `cloud-controls` 返回参数类错误，所需 workspace/设备上下文需在只读契约 fixture 中补齐；在补齐和现场验证前该能力保持 `unverified`。
