## Context

新版 AeroSight 是 pnpm monorepo：Next.js 负责认证、授权、同步数据库操作和 UI，Go Worker 负责 outbox 消费、长连接和后台任务。仓库已经有通用 Connector registry、同步器、`connector_definitions`、`connector_instances` 视图、`connector_sync_runs`、`device_external_identities`、显式绑定、加密凭据和 DJI DeviceType/Driver 产品矩阵。

现有 `dji` Adapter 是平台自建 DJI Cloud API 的直连模式，面向 MQTT、命令和媒体链路。司空 2 OpenAPI 是 DJI 托管公有云的 HTTP 接入模式，认证、能力和网络拓扑不同，必须作为独立 Connector Definition 接入，但最终设备仍进入同一统一设备目录。

已确认中国大陆公有云基地址为 `https://es-flight-api-cn.djigate.com`。项目发现使用 `GET /openapi/v2.0/project`，仅携带 `X-User-Token`；项目设备目录使用 `GET /openapi/v2.0/project/device` 并携带已选 `X-Project-Uuid`。项目列表支持分页，而设备目录不支持分页、单次最多返回 1000 条拓扑；这些差异必须由同一版本化脱敏契约 fixture 固化。

## Goals / Non-Goals

**Goals:**

- 让 owner/admin 只输入一次组织 Token，就能看到可访问项目并选择一个项目完成连接。
- 让 Token 不经过 URL、浏览器持久化或明文数据库，并阻止项目用户控制 Token 的发送目标。
- 复用现有 Connector、同步运行、外部身份、设备类型和租户隔离边界。
- 由 Worker 完成首次、手动和周期设备目录同步，具备响应上限保护、幂等、限流和失败恢复。
- 在 UI 中清楚区分“司空公有云只读同步”和“DJI Cloud API 直连控制”。
- 让连接器页面以已有实例列表为主，只有用户点击“新建连接器”并选择类型后才加载配置向导。

**Non-Goals:**

- 不实现不存在于已确认文档中的 OAuth、授权中继或一键账号登录。
- 不在浏览器调用司空 API，不接收 DJI 用户名、密码或短信验证码。
- 不通过司空连接器开放任务、直播或设备控制，也不把直连 MQTT capability 复制给司空来源。
- 不自动合并来自司空和直连 Adapter 的同一序列号设备。
- 不在本变更实现 EventAPI/Webhook；需要时另行确认签名、重放与事件契约。
- 不开放尚未完成现场设备验收的 DJI Cloud API 直连或其他接入类型；保留其既有后端实现和历史数据，等待后续具备设备时另行启用。

## Decisions

### 1. 司空作为新的 Connector Definition，而不是新的设备子系统

- 注册稳定键 `dji.flighthub2@1.0.0`，发现模式为 `poll`，协议为 HTTPS，兼容现有 DJI 只读 Driver/DeviceType。
- 项目连接继续存入 `device_adapters`，由 `connector_instances` 视图读取；项目 UUID 放入 `discovery_scope_json`，Token 放入 `credential_envelope_json`。
- 设备目录写入 `device_external_identities`，同步过程写入 `connector_sync_runs`；认领后的设备通过现有 binding 和 DeviceType 进入统一目录。
- 为避免同一 AeroSight 项目重复连接同一个司空项目，增加通用 `external_scope_key` 或等效数据库约束，并以 `project_id + connector_definition_id + external_scope_key` 唯一；该键保存项目 UUID 的规范化非秘密表示。

备选方案是新增 `flighthub_connections` 和 `flighthub_devices`。这会复制租户、凭据、同步、设备身份和健康模型，且后续无法与其他 Connector 共用管理页面，因此不采用。

### 2. 使用“两阶段、一次输入”的临时 Token 握手

1. 用户在连接器页输入 Token；React state 保存该值，输入框使用 password 类型并关闭自动完成。
2. 浏览器向项目级 discover Route Handler POST Token。服务端先验证当前用户为项目 owner/admin，再调用项目列表接口；此阶段不写凭据、不写请求体审计、不记录上游完整响应。
3. 服务端只返回项目 UUID、名称和必要的组织显示信息。浏览器保留原 Token 并展示项目下拉框。
4. 用户选择项目并提交 Token + 项目 UUID。服务端重新获取可访问项目，确认所选 UUID 仍在列表内，避免客户端伪造或使用过期发现结果。
5. 服务端在同一事务中创建 Connector Instance、加密保存 Token、写审计摘要并写入首次同步 outbox。成功或失败后客户端清空 Token 和项目列表。

服务端不签发“临时发现票据”，因为票据仍需在服务端暂存 Token 或增加短期秘密状态。当前两次提交同一浏览器内存 Token 更简单，且最终创建时重新验证可以避免 TOCTOU。

### 3. 快捷跳转只做导航和指导，不冒充 OAuth

- UI 提供部署配置允许的司空 2 官方入口，在新窗口打开，并说明组织设置中获取 Token 的位置。
- AeroSight 不拼接 Token 到跳转 URL，不读取 DJI 页面内容，也不声明用户完成了 DJI 账号授权。
- 如果 DJI 未来发布正式授权码/委托授权流程，应新增版本化 credential flow，不复用本 Token 表单模拟 OAuth。

### 4. 上游地址由部署方控制

- `DJI_FLIGHTHUB_API_BASE_URL` 或等效 Worker/Web 共享配置必须是 `https`，启动时校验为部署允许的官方区域主机。
- 项目表单不接受 base URL、代理 URL或设备 API path。企业出口代理使用进程级受控配置，不能由租户指定。
- Web 和 Worker 共用同一版本化 FlightHub client contract、请求头规则、错误码映射和脱敏策略；实现可以分别为 TypeScript 与 Go，但必须使用同一组契约 fixture。
- 请求包含唯一关联 ID、固定超时和有限重试。401/403 不重试；429 尊重有界 `Retry-After`；仅幂等 GET 可重试网络错误和 5xx。

### 5. Web 只负责握手，Worker 负责持久连接后的同步

- 项目发现和最终项目复核必须同步完成，才能形成可理解的两步交互，因此由 Next.js Route Handler 调用司空项目 API。
- 创建后的首次同步、立即同步和周期同步一律写入 outbox，由 Worker 领取 Connector 租约后执行；HTTP 请求不得等待完整设备目录同步。
- Worker 在启动时注册 `dji.flighthub2` runtime，并把通用 Connector 调度器接入现有运行组。调度器按 `connector_definition_id`、状态和租约领取实例，保证同一实例单活。
- 手动同步使用幂等键合并并发请求；周期同步使用抖动和指数退避，避免多个项目同时击穿司空限流。

### 6. 只有完整且未触及上限的设备目录才能提交完整快照

- Worker 使用已选 `X-Project-Uuid` 调用单次设备目录接口。只有请求成功、响应少于 1000 条官方上限且通过 schema 校验时，才返回 `CompleteSnapshot=true` 并推进同步游标。
- 请求失败、响应达到 1000 条且无法证明完整性、响应超限或 schema 不兼容时，同步运行记为失败/退避，不推进游标、不部分提交设备，也不把未出现的身份标记为 `missing`。
- 外部身份键使用 `projectUuid + deviceSn` 的稳定规范化值；机场与飞行器父子关系通过 `ParentExternalID` 表达。
- 重复 SN、跨项目返回、缺失 SN 或未知嵌套结构进入隔离诊断，不覆盖其他项目或其他 Connector 的身份。

### 7. 司空来源默认只读，能力不会从直连模式继承

- 已知 Dock 2/3 和配套飞行器可匹配现有 DeviceType，但该 Connector 只声明已实现的 inventory/status 能力。
- DeviceType/Driver 的静态能力仍要与 Connector runtime availability 求交集；没有司空下行实现时，任务、返航、机场调试和直播 action 必须被移除。
- 默认纳管策略为 `review`。只有唯一、已验证的产品类型匹配且管理员明确启用 `automatic` 时才能自动创建设备；未知型号保持 `discovered` 或 `conflicted`。
- 同一 SN 已被 `dji` 直连 Adapter 管理时不得静默合并。系统展示冲突来源并要求管理员选择绑定策略；本变更不改变既有下行主路由。

### 8. 凭据、错误和审计均按最小披露设计

- Token 使用现有 AES-GCM credential envelope、随机 nonce、key version 和包含 project/adapter 的 AAD；数据库只保存密文 envelope。
- Token 不得出现在 URL、Cookie、localStorage/sessionStorage、React Server Component props、错误消息、审计 input、日志、trace、metrics label 或测试快照。
- 审计只记录发现成功/失败、返回项目数量、所选项目 UUID 的脱敏形式、连接器 ID和同步请求，不记录项目列表原始响应。
- UI 只展示归一化错误：Token 无效、无项目权限、限流、区域配置错误、上游不可用或响应不兼容。

### 9. 生命周期与断开语义

- 更新 Token 时先用新 Token 复核当前项目，成功后原子替换 envelope 并触发同步；失败时保留旧 Token 和最近成功快照。
- 断开时禁用 Connector、撤销租约并停止新同步。已有 Device、事件和审计历史保留，binding 标记不可用，设备状态按现有新鲜度规则收敛，不能级联删除资产。
- 删除实例仅在现有 Connector 生命周期允许且明确二次确认时执行；默认 UI 使用“断开/禁用”而不是硬删除。

### 10. 连接器页面采用列表优先与按需创建流程

- 服务端页面加载当前项目的已有 Connector Instance，并将类型、外部作用域、状态、最近同步和安全摘要投影为列表行；`disabled`、`failed` 等实例不会因为不可继续同步而从列表隐藏。
- owner/admin 在页面右上角点击“新建连接器”后进入类型选择；首版 UI 类型目录只注册已经完成本变更验收的 `dji.flighthub2`，选择后才挂载现有两阶段 FlightHub 向导。
- 现有 `dji` Cloud API 直连组件不再由连接器页常驻渲染，也不作为可选类型。相关 Route Handler、数据库记录和后端代码不删除，以免破坏历史实例或扩大本次范围。
- 类型选择与配置向导使用页面内可关闭的对话框或等效覆盖层；关闭、成功或卸载时销毁组件，使内存中的 Token 与临时项目列表随组件一起清理。
- member 沿用现有策略，不能进入连接器管理页面；权限检查发生在列表和“新建连接器”渲染之前。

备选方案是继续让每种连接方式以内联大卡片垂直堆叠。这会让空配置表单压过已有实例，且接入类型增加后难以浏览，因此不采用。

## Risks / Trade-offs

- [司空区域、套餐或 OpenAPI 版本差异] → 部署固定区域主机，客户端按版本化 fixture 实现；未知 schema 失败关闭，不猜测字段。
- [组织 Token 权限过大或撤销不及时] → 最小权限说明、加密存储、更新/断开入口、401/403 立即降级并停止无意义重试。
- [项目发现接口可被滥用探测] → owner/admin 权限、每用户/项目速率限制、超时和审计摘要。
- [设备目录无分页且最多返回 1000 条] → 少于上限才视为可证明完整的目录；达到上限、超限或响应过大时失败关闭并告警，不处理 missing。
- [同一设备被司空与直连 MQTT 同时发现] → 独立 ExternalIdentity、冲突状态和人工绑定，不自动改变控制路由。
- [Worker 周期同步造成限流尖峰] → 单实例租约、抖动、指数退避、`Retry-After` 和手动同步幂等合并。
- [两阶段提交使 Token 在浏览器内存停留更久] → 不持久化、离开/取消/成功/失败即清空；不使用服务端临时 Token 缓存以减少另一类秘密状态。

## Migration Plan

1. 增量注册 `dji.flighthub2` Connector Definition，并增加通用外部作用域唯一约束；迁移不得改变现有 `dji` Adapter。
2. 部署 Web 项目发现/创建 API 和只读 UI，功能开关默认关闭；确认响应和审计完全脱敏。
3. 部署 Worker runtime、调度器和契约 fixture，在 mock server 完成设备目录上限、限流、撤权和重启恢复测试。
4. 在隔离 AeroSight 项目连接司空测试组织，先以 `review` 模式验证项目列表、现有 Dock/飞行器身份和重复同步；未具备的现场硬件使用版本化契约 fixture 验证失败关闭与只读边界。
5. 完成真实区域、套餐、列表式管理页面和可用类型目录验收后启用生产功能开关；自动纳管仍默认关闭，其他接入类型保持不可创建。

回滚时先关闭司空连接器创建入口和周期调度，再禁用现有实例。数据库中的 Connector Definition、同步运行、外部身份和加密 envelope 保留供审计；旧版本 Web/Worker忽略新 definition，不删除已纳管设备。

## Open Questions

- 司空设备目录能否稳定返回 Dock 与飞行器拓扑、型号和在线状态，需要用真实响应脱敏 fixture 验证；缺失字段时应保持待确认而不是推断。
- 如果未来需要司空侧任务、直播或事件推送，应分别基于官方接口与权限新增 Change，不扩大本只读连接器的隐含权限。
