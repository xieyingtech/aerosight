## Why

AeroSight 已支持平台自建 DJI Cloud API MQTT 接入，但这种模式要求管理员配置 Broker、应用凭据、媒体网关和机场序列号。对于已经接入 DJI 司空 2 公有云的机场，用户更希望复用司空组织令牌，让 AeroSight 自动列出该令牌可访问的项目，选择一个项目后直接同步机场与飞行器目录，而不是重新配置一套设备侧网络。

DJI 当前公开的司空 2 OpenAPI 组织令牌模式不是 OAuth 授权码流程，因此不能提供真正的“登录 DJI 并授权”回调。首版应把手工取 Token 的步骤缩短为一次输入，并提供打开司空 2 的快捷入口；Token 验证成功后自动发现项目，用户只需选择目标项目完成连接。

## What Changes

- 注册 `dji.flighthub2` Connector Definition，复用现有 `device_adapters`/`connector_instances`、加密凭据、同步运行、外部身份和设备绑定模型，不新增平行的司空专用连接表。
- 在项目“连接器”页增加司空 2 两阶段向导：用户输入一次组织 Token，服务端仅使用 `X-User-Token` 获取可访问项目，用户选择项目后服务端再次验证并创建连接器实例。
- 提供“打开司空 2 获取 Token”的新窗口快捷入口和清晰步骤说明，但明确它不是 DJI OAuth，AeroSight 不接收 DJI 账号密码。
- Token 在项目发现阶段只存在于当前浏览器表单内存和单次服务端请求中；完成连接后使用现有凭据 envelope 加密持久化，任何响应、日志、审计输入和遥测均不得包含 Token。
- 将司空 OpenAPI 基地址限定为部署方配置的官方区域地址；项目用户不能提交任意上游 URL，避免 SSRF 和令牌外送。
- 创建连接后通过 outbox 请求首次同步；Go Worker 注册司空 `poll` runtime，单次获取所选司空项目最多 1000 条设备拓扑，完整校验后写入现有 `device_external_identities` 和 `connector_sync_runs`。
- 已确认的 Dock 2/3 与配套飞行器继续使用现有 DJI DeviceType/Driver 产品矩阵；未知型号保持待确认或只读，不因来自司空而自动获得控制能力。
- 提供连接状态、最近验证、最近同步、同步数量、脱敏错误、立即同步、更新 Token 和断开操作；所有管理操作仅允许项目 owner/admin。
- 将项目“连接器”页改为列表优先的管理页面：默认展示已有连接器实例，owner/admin 点击“新建连接器”后先选择类型，再进入该类型的配置流程。
- 当前发布只开放已完成无设备依赖验收的 `dji.flighthub2` 创建入口；DJI Cloud API 直连和其他依赖现场设备的接入方式暂不作为可选类型，也不再以内联表单常驻页面。
- 司空连接器首版为只读目录同步。远程任务、设备控制、直播、遥测订阅和 EventAPI 只有在对应官方接口、权限与契约另行确认后才能扩展，不复用直连 MQTT 能力做虚假承诺。
- 非目标：不实现 DJI OAuth、账号密码托管、浏览器直连司空、任意 OpenAPI URL、司空项目自动跨 AeroSight 项目共享、司空与直连 Cloud API 的自动合并，也不在本变更中新增或开放依赖物理设备验收的接入与控制入口。

## Capabilities

### New Capabilities

- `dji-flighthub-connector`: 定义司空 2 Token 项目发现、项目选择、加密连接、Worker 设备同步、连接生命周期、权限和安全降级行为。

### Modified Capabilities

无。当前主规格目录尚未同步连接器能力；本变更基于仓库中已实现的通用 Connector 和统一设备平台代码，不复制其领域模型。

## Impact

- Web/API：扩展 `apps/web` 的连接器列表、类型选择流程、Adapter policy、项目级 Route Handler、审计和凭据 envelope 使用方式；项目发现是唯一允许同步调用司空 OpenAPI 的临时握手路径。
- Worker：在 `apps/worker` 注册司空 HTTP 客户端和 Connector runtime，接入现有 outbox、租约、同步游标、完整快照和退避机制。
- 数据库：为 Connector Definition 增加司空种子记录，并为通用 Connector Instance 增加可约束的外部作用域键或等效唯一性约束；不新建 Token 表、设备表或事件表。
- 设备模型：复用 `device_external_identities`、`device_connector_bindings`、DeviceType、Driver 和能力求交集；同一序列号不会因重复同步重复创建设备。
- 安全：复用现有 AES-GCM credential envelope 和 AAD；新增 Token 泄漏、SSRF、跨项目、重放和错误脱敏测试。
- 运维：部署方必须配置目标区域的官方司空 OpenAPI 基地址、请求超时、同步周期和出口 HTTPS；当前交付以真实司空目录和版本化 fixture 完成验收，不要求补齐仓库团队并不具备的 Dock 3 等现场硬件。
- 兼容性：现有 `dji` 直连 Cloud API Adapter 不迁移、不改变；同一物理设备同时来自两种连接器时进入身份冲突/人工认领流程，不自动合并或切换下行主路由。
