## Context

参见 [proposal.md](./proposal.md) 的动机与范围。当前数据库已经存在通用设备、Adapter、命令、直播和算法领域表的早期结构，Go 包也有 DJI 消息转换与回调安全辅助代码，但没有生产 MQTT 会话、完整 Topic 路由、实机命令闭环或媒体网关；Web 的 DJI 连接测试仍明确返回未实现。现有演示器只向标准输出写事件，未经过 Adapter 摄取边界，因此无法驱动页面。

本设计需要同时覆盖 Dock 2/Matrice 3D 系列和 Dock 3/Matrice 4D 系列、局域网与公网部署，并保持 User → Team membership → Project 的租户边界。DJI 产品枚举、命令与直播源存在代际差异，必须通过版本化产品能力矩阵处理，不能散落在 UI 条件分支中。

## Goals / Non-Goals

**Goals:**

- 用同一领域设备模型管理机场、飞行器、机器人、摄像头、传感器和未来硬件，以 DeviceType → Driver 决定协议与基础能力，以设备关系表达拓扑。
- 形成可在协议演示器和真实 Dock 上复用的 MQTT 摄取、命令、视频/遥测/传感实时数据和状态恢复路径。
- 让单机局域网演示部署尽可能简单，同时让公网部署具备独立端点、TLS、鉴权和秘密管理边界。
- 将算法系统收敛为通用能力目录，任何具体识别任务只是项目数据和映射配置。
- 先交付可审计的离散命令和航线任务，不把浏览器变成实时飞控器。

**Non-Goals:**

- 不抽象到“所有硬件完全相同”；统一的是 Device 生命周期和授权边界，设备差异仍由 DeviceType、Driver manifest 和类型化 capability schema 保留。
- 不在本变更内实现任意连续 DRC/摇杆控制、全套机场维护命令、视频转码器或模型训练。
- 不保证无公网入口、NAT 穿透或证书条件的现场能由平台自动修复网络。
- 不使用算法回调或大模型输出直接触发物理设备动作。

## Decisions

### 1. 所有可独立识别硬件都是 `devices`

- `devices` 只保存实例身份、`device_type_id`、显示名称、当前状态和项目归属；直接或继承的接入路由由显式 Connector 绑定表达。无人机、机场、机器人、摄像头和传感器使用同一张表，不存在 `device_components` 特殊分支。
- `device_relationships` 表达 `gateway-for`、`docked-aircraft`、`mounted-on`、`contains` 等版本化设备关系，并以复合约束确保关系两端同项目。通过父设备通信的相机或传感器仍是 Device，只是路由经网关关系解析。
- 视频镜头、温度读数等不是新的硬件身份，而是设备 Driver 暴露的带稳定 channel ID 的 capability；相机和温度传感器本身仍是 Device。
- 厂商原始字段保存在受限扩展 JSON/原始消息对象中，设备目录和业务查询不依赖 DJI 专表。

选择理由：凡是需要状态、实时数据、审计或权限的实体都使用相同设备边界，RBAC 不再需要“设备与组件”两套规则。仅把数据通道保持为 capability 可以避免将一个多镜头相机错误拆成多台硬件设备。

### 2. DeviceType 绑定 Driver，Connector 只承载项目接入配置

- `driver_definitions` 是系统注册的版本化驱动目录，主键语义为 `driver_key + semver`；manifest 声明支持的协议、可发现类型、基础 capability schema、状态映射、命令处理器和实时流处理器。Driver 是 Worker 中的实现插件，不保存项目秘密。
- `device_types` 是版本化设备型号/类型目录，包含稳定 `type_key`、厂商/型号元数据、`driver_key`、兼容 Driver 版本范围及类型能力 profile。每个 Device 必须引用一个有效 DeviceType。
- `connector_definitions` 是平台级、版本化的连接器类型目录，声明配置/凭据 schema、发现模式、支持协议、健康检查和兼容 Driver 范围，不保存项目秘密。DJI Cloud API、MQTT、AWS IoT Core、Azure IoT Hub、ThingsBoard、OPC UA、ROS 2、GB28181 等均以 ConnectorDefinition 扩展，而不是在设备页面增加厂商字段。
- `connector_instances` 是项目级连接器运行实例，保存 ConnectorDefinition 版本、扫描范围、网络 profile、凭据引用、同步策略、租约和健康状态。同一个 DJI Connector 可经多个机场网关管理多台 Device；设备可直接绑定 Connector，也可通过网关关系继承路由。
- `device_capabilities` 是计算后的有效投影：`Driver manifest ∩ DeviceType profile ∩ firmware/device report ∩ runtime availability`。后三者只能收窄、禁用或参数化 Driver 已知能力，不能动态增加未注册 action。
- Driver 升级只允许落入 DeviceType 的兼容范围；升级和 Connector 重配不改变 Device ID。超出范围时先发布新 DeviceType 版本并显式迁移设备。

备选方案是让每台 Device 直接绑定任意 Driver；配置灵活但同一型号会产生不一致能力和协议选择，也难以统一升级，故 Driver 必须由 DeviceType 决定。另一个备选是让 DeviceType 保存项目凭据，会把全局类型目录和租户秘密耦合，故连接配置单独放在 Connector 实例。

### 3. Connector 负责发现和传输，Driver 负责设备语义

- ConnectorDefinition 负责如何连接一个外部 IoT 平台以及如何取得设备目录，发现模式支持 `push`、`poll`、`subscribe` 和 `manual-import`；ConnectorInstance 负责某项目中的端点、租户/workspace、Topic、扫描范围、同步游标、凭据和自动纳管策略。
- `device_external_identities` 保存连接器发现的原始身份、厂商类型、属性、拓扑、首次/最后发现时间和来源版本，生命周期为 `discovered`、`managed`、`ignored`、`conflicted` 或 `missing`。重复扫描只更新外部身份，不重复创建 Device。
- `device_connector_bindings` 显式连接 Device、ConnectorInstance 和 ExternalIdentity，并保存 `direct`、`gateway` 或 `inherited` 路由角色、优先级和绑定状态。一个 Connector 可绑定多个 Device；同一 Device 可在迁移或主备场景保留多个绑定，但同一时刻只允许一个明确的下行主路由。
- 连接器先归一化外部 envelope 和拓扑，再由兼容 Driver/DeviceType 解析设备语义、能力和通道。连接器不得自行发明 capability，Driver 也不得读取项目秘密或自行扩大扫描范围。
- 自动同步仅处理 ConnectorInstance 明确配置的外部范围。纳管策略支持 `automatic`、`review`、`observe-only`：只有外部身份唯一、DeviceType 匹配明确且无冲突时才可自动创建 Device；其余对象进入设备页待确认区。
- 外部对象暂时未被扫描到时先标记 `missing` 并保留 Device、历史和绑定；达到连接器定义的退役阈值后只能由显式策略归档，绝不因一次扫描缺失而删除设备。

设备页面是统一设备目录和发现结果入口，展示已纳管、待确认、冲突和来源缺失对象；连接器页面管理实例、端点、凭据、扫描策略、健康状态和同步日志。Connector 本身不是 Device，除非外部平台报告的网关硬件具有独立身份、状态和能力，此时该网关硬件另建 Device。

现有 `device_adapters` 可作为 `connector_instances` 的兼容迁移来源或物理表名暂时保留，但新领域类型、API 和 UI 统一使用 Connector 命名；迁移期间不得同时建立语义重叠的 Adapter 与 Connector 两套运行实例。

### 4. RBAC 使用设备范围 + capability action

- 团队 owner/admin/member 继续提供项目默认权限；细粒度 grant 由 `subject + project_id + device_scope + action_pattern` 组成。`device_scope` 支持项目全部设备、指定 DeviceType 或指定 Device。
- capability action 使用稳定命名空间，例如 `state.read`、`stream.telemetry.read`、`stream.sensor.read`、`stream.video.read`、`stream.video.control`、`mission.execute`、`motion.control`、`dock.cover.control`、`sensor.configure`。
- API 在每次读取、订阅、配置和命令时用目标设备的有效能力重新解析 action 并鉴权；UI 隐藏按钮只改善体验，不是授权边界。Driver/Worker 在投递前再次校验授权快照、能力和安全策略。
- DeviceType 范围授权只匹配显式类型版本族，不能因设备被错误改型而静默扩大权限；类型迁移会触发授权影响预览和审计。

选择理由：只按“设备可读/可控”授权无法区分看实时数据、启动视频、执行任务和修改传感器配置。能力 action 与 Driver manifest 同源，可以复用在 API、UI、命令账本和审计中。

### 5. DJI Driver 是 Go Worker 中的长连接边界

- 每个启用的 DJI Connector 实例由 Worker 中的 DJI Connector runtime 以租约单实例持有，建立 MQTT 5 会话并订阅扫描范围内网关序列号的状态、事件、请求和服务回复 Topic；多 Worker 通过数据库租约避免重复活跃连接。
- 上行首先保存最小原始 envelope/对象存储引用，再调用兼容 DJI Driver 的 topology/translation 逻辑转换为统一事件。幂等键由 Connector 实例、网关序列号、消息 ID/业务 ID 和方法组成，不能只依赖到达顺序。
- 下行只读取已经进入 `dispatchable` 的 `device_commands`，由能力映射表选择 DJI method/topic，生成厂商业务 ID并写入 `command_attempts`；回复按网关、业务 ID 与 method 三元组关联。
- 断线重连使用持久会话或重新订阅后状态同步；无法判断结果的非幂等命令标记 `unknown` 等待人工核查，不能盲目重发。

备选方案是在 Next.js Route Handler 中直接连接 MQTT；长连接、恢复和后台消费不适合请求生命周期，且会模糊同步授权与异步设备面的边界，故不采用。

### 6. 产品能力矩阵生成 DeviceType 并隔离 Dock 2/3 差异

- 在 DJI Driver 中维护受测试保护的 `vendor=dji + domain + type + subtype + firmware/api range` 产品矩阵，注册机场、飞行器、相机和可独立识别传感器 DeviceType，并输出拓扑角色、状态映射、实时通道和 capability profile。
- 矩阵和 DeviceType 按版本注册，并保留设备最后解析版本；新固件或未知枚举先进入 `degraded/read-only`，通过契约夹具验证后再开放控制。
- Dock 2/Matrice 3D/3TD 与 Dock 3/Matrice 4D/4TD 使用独立夹具和映射测试，禁止通过名称前缀猜测能力。

选择理由：单一巨大 switch 容易在新型号上静默套用旧枚举。将差异集中在 Driver 提供的 DeviceType/能力矩阵，可以让 UI 和任务层保持厂商无关，也便于以后新增产品或 Driver。

### 7. 网络使用一个部署 profile、四类外部端点

ConnectorInstance 引用一个不可变/版本化连接 profile：

- `mqtt_endpoint`：机场/模拟器可达的 Broker 地址、TLS、client/tenant 认证引用。
- `api_public_base_url`：DJI WebView、设备或 callback 可达的 HTTPS 地址。
- `websocket_public_url`：需要实时协商或 DJI Pilot 接入时的外部地址。
- `media_ingest_base_url` 与 `media_playback_base_url`：分别面向设备推流和用户播放，可不同域名/网段。

`lan` profile 允许 RFC1918 地址，但拒绝 `localhost`/回环作为设备端点；`public` profile 强制可信 TLS、关闭匿名 Broker、限制 Topic ACL，并要求轮换秘密。连接向导执行 DNS、TCP/TLS、Broker 认证、HTTP health 和媒体 publish/play 探测，无法从服务端证明设备侧可达时明确标为“待现场验证”。

备选方案是根据浏览器访问 Host 自动推导所有端点；在 NAT、反向代理和双网卡环境下不可靠，故端点必须显式配置。

### 8. 演示使用 MQTT Broker + MediaMTX，实时数据接口保持可替换

- 本地/现场演示 compose 增加支持 MQTT 5 和 ACL 的 Broker，以及 MediaMTX。DJI 优先推 RTMP 到 MediaMTX，浏览器优先 WebRTC、回退 HLS；不把媒体字节经过 Web/数据库。
- `device_stream_channels` 保存 Driver 为设备声明的通道 ID、数据类型、schema、单位、质量和当前可用性；视频会话只保存控制状态和不含永久秘密的 locator，遥测/传感通道复用实时事件管线与游标，不复制设备身份。
- Worker 创建随机流 key、调用 DJI `live_start_push` 类服务并轮询/接收回复；媒体网关 webhook/控制 API 确认真实输入后才将会话标记 `live`。停止命令和网关清理由协调器幂等收敛。
- Web 在设备范围与 capability action 授权通过后返回短期播放凭据或建立遥测/传感订阅。公网 MediaMTX 位于反向代理后，并限制发布端点与播放端点权限。

选择理由：RTMP 摄取兼容范围较清晰，MediaMTX 能用单一小型组件提供 HLS/WebRTC。直接在 Next.js 转流会造成资源和安全问题；只把厂商 RTMP URL交给浏览器通常不可播放，均不采用。

### 9. 协议演示器复用真实 Connector/Driver，而不是写数据库

- 新演示器扮演 Dock/Pilot 侧客户端，连接同一个 MQTT Broker，发布 Dock 2 或 Dock 3 官方形状的拓扑、状态和事件，并消费平台服务 Topic 返回 ACK/NACK/延迟/超时；平台侧数据必须经过真实 Connector 和 Driver 边界。
- 视频使用两个可辨识的循环测试源代表相机设备，并模拟温度/环境传感器设备的时序采样；所有数据通过 DJI Connector/Driver 的统一协议路径进入平台。
- 场景文件控制固件、轨迹、掉线、互锁和失败注入。演示器不得直接调用内部数据库或业务 API来伪造成功，这使协议路径成为本地验收测试。

选择理由：现有 stdout simulator 只能测试事件生成，不能证明 Broker 认证、Topic 路由、命令关联或直播链路。协议级模拟器覆盖的风险更接近实机。

### 10. 算法服务使用通用 Provider/Definition/Run 三层模型

- `algorithm_providers` 只保存传输与运行策略：adapter type、base URL、secret ref、health、timeout、并发、allowlist。
- `algorithm_definitions` 与不可变 version 保存名称、能力类别、输入 JSON Schema、参数 JSON Schema、输出映射 DSL、标签/展示元数据；“疑似违建”作为 seed/template 数据而非前端常量。
- `algorithm_runs` 引用定义版本与不可变输入资产版本。Adapter 将 HTTP JSON 等协议转换为 canonical result union：classification、detection、segmentation、keypoints、tracking、OCR、scalar/table、asset/custom。
- UI 用服务端 schema/catalog 渲染列表、参数表单和结果组件；无法标准化的输出保留原始资产引用并明确为 `partial`/`mapping_required`。
- Provider callback 只完成算法运行。任何事件或设备后续动作都必须经规则、任务草案、用户确认和统一命令账本。

备选方案是为每个算法类型写独立页面和字段；短期直观但每接一个算法都要发布前后端，也容易把模型输出误当业务结论，故不采用。

### 11. 数据迁移保持向后兼容和项目隔离

- 使用只增不删迁移：注册兼容 legacy/unknown Driver 和 DeviceType，为现有 `devices` 回填 `device_type_id`；将现有 Adapter 运行实例迁移或兼容映射为 ConnectorInstance，将 Adapter 外部身份迁入 Connector 唯一身份和显式绑定结构，并补充设备关系、有效能力、实时通道、细粒度 grant 与网络 profile。
- 复用现有命令、直播和算法表时优先增加版本、状态或 schema 字段，不复制第二套平行账本。旧的 `dji` adapter type 仍可读，但只有通过新 profile 验证后才能启用生产连接。
- 所有项目级新表直接含 `project_id`，设备关系使用 `(project_id, id)` 复合外键或等效事务校验。Driver/Connector 定义和系统 DeviceType 可为全局只读目录，项目自定义类型必须绑定团队/项目管理权限；Provider、实时订阅、命令和 Connector 配置的每次读取都从已授权项目开始。
- 迁移不会自动为历史“违建算法”赋予法律结论；它被转换为普通 definition/category 与现有结果来源。

### 12. 控制安全以离散能力和命令账本为边界

- 第一阶段 Driver 白名单包含航线任务投递/开始/取消、安全返航，以及 DeviceType 支持的机场调试命令；每项 capability 定义独立 action、风险级别、状态互锁、二次确认和超时策略。
- 高风险命令在 Web 和 Worker 双重校验；UI 确认不是安全边界。紧急类命令可提高优先级但仍必须入账并等待确认。
- 任意连续 DRC 控制需要独立的低延迟链路、控制权租约、dead-man switch 和更严格现场安全设计，留给后续 Change。

选择理由：最小实机演示首先要可控、可审计、可恢复。直接暴露厂商 Topic 或通用“调用 method”接口会绕过能力限制并扩大事故面。

## Risks / Trade-offs

- [DJI 固件、账号许可或 Cloud API 枚举变化导致实机不兼容] → 锁定并展示兼容矩阵，保存原始消息，以 Dock 2/3 真机契约夹具和只读降级保护控制面。
- [局域网服务端自检不能证明机场到端点的单向可达性] → 提供设备侧上线检查清单和待现场验证状态，以实际 MQTT/RTMP 握手作为最终就绪信号。
- [公网暴露 MQTT 与媒体发布端点扩大攻击面] → TLS、每 ConnectorInstance 独立凭据、Topic ACL、随机流 key、速率限制、网络 allowlist 与秘密轮换。
- [同一机场同时只能推有限路数或占用相机资源] → 能力声明并发限制、会话租约和冲突提示；不承诺所有型号同时双路直播。
- [MediaMTX WebRTC 受企业网络或 NAT 限制] → 允许配置 ICE/TURN 并回退 HLS，UI 展示实际协议和延迟。
- [统一设备模型变成最低公分母] → 只统一 Device 生命周期、拓扑、状态和命令/实时流 envelope，差异保留在 DeviceType 与版本化 Driver capability manifest 中。
- [错误 DeviceType 或 Driver 升级可能开放不安全能力] → 类型版本不可变、Driver 兼容范围、有效能力求交集、未知能力隔离、升级影响预览和控制回归测试。
- [细粒度 RBAC 增加查询复杂度] → 角色提供默认授权，grant 只存例外；缓存按用户/项目/设备/能力版本失效，Worker 投递前仍执行最终校验。
- [动态算法 schema 使 UI 渲染复杂] → 首期限制支持的 JSON Schema 子集和标准结果 union，未知类型采用只读 JSON/资产展示而非猜测语义。
- [协议模拟成功不等于实机飞行安全] → 将模拟、联调和实机验收分级展示；实机控制仍要求现场清单、兼容固件、空域与人工确认。

## Migration Plan

1. 部署只增不删的 Driver、DeviceType、设备关系、有效能力和 capability grant 迁移，使用 legacy 类型回填历史设备并运行租户隔离/授权回归。
2. 部署 MQTT Broker、MediaMTX 与网络 profile 校验，默认不开放公网匿名访问；Worker Connector 功能开关保持关闭。
3. 接入协议演示器，分别完成 Dock 2 和 Dock 3 的 DeviceType 发现、状态、命令成功/NACK/超时、摄像头视频与传感实时数据验收。
4. 启用设备类型/Driver、能力驱动设备 UI、视频/遥测/传感实时面板和动态算法目录，移除页面内的具体设备/算法类别常量，以种子数据保留可选示例。
5. 在隔离项目中配置 DJI 开发者信息和局域网 profile，先只读接入 Dock 2/3，再逐项开启直播、机场调试和航线任务。
6. 公网试点启用受信域名、TLS、Topic ACL、媒体发布/播放鉴权和秘密轮换，通过外部网络连通性与安全测试后放量。
7. 完成两代机场的实机证据记录后再默认展示相应控制能力；未经验证的固件保持只读。

回滚时先关闭所有项目的 DJI 下行命令与新直播创建，停止 Connector 租约并让活动命令进入可审计的暂停/未知状态；随后回滚 Web/Worker。新增表列和历史事件保留，旧版本忽略新结构；媒体网关停止新发布但不立即删除已形成的证据资产。

## Open Questions

- 试点公网域名、证书签发方式、MQTT Broker 发行版和 TURN 服务由部署环境决定；接口与验收行为不依赖具体供应商。
- 实机验收时使用的具体固件版本和 DJI 开发者账号授权将在部署记录中固化，不改变两代产品族和只读降级要求。
