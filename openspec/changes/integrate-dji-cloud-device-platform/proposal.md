## Why

AeroSight 现有 DJI 代码尚未连接真实 Cloud API，设备与直播也没有形成可操作闭环；同时部分页面仍把无人机、机场或特定算法当作固定业务对象，无法支撑后续接入其他厂商设备和识别能力。现在需要先交付可由协议演示器和版本化 fixture 验证的统一设备、连接器与算法平台；由于当前没有 Dock 2/3 现场设备，直连接入与物理控制入口必须保持关闭，实机启用留待具备设备后另行验收。

## What Changes

- 建立统一设备层：无人机、机场、摄像头、机器人、传感器及其他可独立识别实体均作为项目级设备管理，不再使用特殊组件模型；每个 Device 必须引用一个 DeviceType，DeviceType 绑定版本化 Driver，Driver 决定基础能力和协议实现，设备拓扑只表达设备之间的父子、挂载、网关或停靠关系。
- 建立通用 Connector 接入层：ConnectorDefinition 定义 IoT 平台的配置、鉴权、发现、同步与健康检查契约，项目级 ConnectorInstance 保存端点、凭据引用、扫描范围和自动纳管策略；连接器发现的外部身份经过类型匹配与显式绑定进入统一 Device 目录。
- 实现 DJI Cloud API Connector 的协议、产品矩阵和安全边界，覆盖 Dock 2 + Matrice 3D/3TD 与 Dock 3 + Matrice 4D/4TD 的自动发现、状态/遥测、能力映射、命令下发、回复关联、掉线与降级处理；当前只通过协议演示器和 fixture 验证，不在连接器类型目录开放直连接入或物理控制。
- 交付最小安全控制集：任务投递/取消、返航及经过权限和安全确认的机场远程调试命令；所有命令进入统一命令账本，不允许 UI 直接发布厂商 Topic。
- 建立通用实时流能力：支持设备通过 `stream.video`、`stream.telemetry`、`stream.sensor` 等能力暴露实时通道；视频经媒体网关摄取并转换为浏览器可播放的 HLS/WebRTC，遥测和传感数据使用统一实时订阅，页面不写死无人机、机器人、摄像头或传感器展示类型。
- 同时支持局域网和公网部署拓扑，分别提供可配置的 MQTT、HTTPS、WebSocket、RTMP 与播放地址、TLS/鉴权要求和连通性检查，不把开发机地址暴露为生产默认值。
- 将算法服务重构为通用 Provider、Definition、内部 Configuration Snapshot、Run 和结果映射模型；算法模型版本由 Provider 管理，平台仅记录 Provider 返回的 model revision/digest 与每次运行采用的不可变接入配置快照。UI 允许直接保存并立即使用算法配置，不提供模型版本发布流程；能力目录仍从服务端动态渲染且不写死“违建”算法，违建识别仅作为可选示例定义/模板。
- 增加可在无实机时运行的 DJI 协议级演示器，模拟 Dock 2/3 拓扑、遥测、命令回复和双路视频源；实机验收仍要求合法 DJI 开发者配置、兼容固件与可达网络。
- 连接器管理采用列表优先和按需创建流程；类型目录只暴露已完成当前交付与验收门槛的类型。当前仅开放 `dji.flighthub2`，其他依赖现场设备的接入保留代码和历史实例但不可新建。
- 保持现有项目、设备、任务、资产、事件和算法数据兼容；仅使用增量迁移，并为历史设备/算法记录提供默认类型或回填策略。
- 非目标：本变更不实现任意摇杆/DRC 连续飞控、全量 DJI Cloud API、视频编解码器自研、自动绕过现场安全检查、其他厂商生产 Connector 或算法训练平台；通用 Connector 契约和非 DJI fixture 属于范围，但其他 IoT 平台的生产协议实现留给后续变更。

用户将能够在同一个设备目录中查看机场、无人机、机器人、摄像头和传感器的拓扑与实时状态；系统依据设备类型所绑定 Driver 的能力动态提供控制、视频或实时数据入口，并从通用算法目录配置和运行不同识别服务。

## Capabilities

### New Capabilities

- `unified-device-platform`: 项目级统一设备、ConnectorDefinition/ConnectorInstance、自动发现与纳管、DeviceType → Driver 绑定、设备拓扑、外部身份与绑定、有效能力、能力级 RBAC、状态投影及厂商无关的命令入口。
- `dji-cloud-control`: Dock 2/3 与配套飞行器的 Cloud API 连接、遥测摄取、命令映射、回复关联、安全控制和协议级模拟验收。
- `live-stream-gateway`: 厂商无关的实时通道发现与授权订阅，包括视频直播、DJI 推流控制、RTMP 摄取、HLS/WebRTC 播放以及遥测/传感数据实时查看。
- `algorithm-service-platform`: 通用算法 Provider、可直接保存的算法定义、内部配置快照、Provider 模型 revision 追溯、输入输出契约、运行调度、结果映射和动态 UI 能力目录。

### Modified Capabilities

当前 `openspec/specs/` 下没有已归档能力规格，本变更不修改既有主规格。

## Impact

- 数据库：增量建立 ConnectorDefinition、ConnectorInstance、发现同步状态、外部身份、显式设备绑定、Driver 定义、版本化 DeviceType、设备拓扑、有效能力和能力级授权，补齐 DJI 会话和实时通道/直播会话信息，并确保算法 Provider、Definition、内部 Configuration Snapshot、Run 不依赖业务类别；项目资源继续携带并校验 `project_id`。
- Worker：新增通用 Connector 租约、发现/增量同步和身份归一化边界，以及 DJI MQTT 长连接、Topic 路由、HTTPS/WebSocket 辅助接口、命令回复处理、连接恢复、直播控制和协议模拟器；沿用 outbox、幂等和命令账本边界。
- Web/API：新增独立连接器管理、健康/同步日志、发现设备处理入口，以及通用设备拓扑、Driver/DeviceType、能力驱动操作、视频/遥测/传感实时面板和通用算法管理/运行界面；设备页面从连接器自动发现结果形成统一目录，所有查看、订阅和控制操作按项目、设备范围与 capability/action 重新鉴权。
- 基础设施：开发/演示部署增加 MQTT Broker 和 MediaMTX 等媒体网关；公网部署要求域名、TLS、受限端口、秘密引用和设备可达性检查，局域网部署允许显式配置可路由的内网地址。
- 外部依赖：生产启用 DJI Cloud API 直连时需要应用配置、支持的 Dock/飞行器固件和账号授权；当前无实机交付不把这些依赖伪装为已满足。
- 兼容性：已有设备和算法记录不删除，旧设备类型迁移为绑定兼容 Driver 的版本化 DeviceType；无法解析类型或 Driver 的设备以 `unavailable`/只读状态保留，尚未配置 MQTT/媒体依赖的部署显示 `unavailable` 或 `degraded`，不伪装为已连接。
