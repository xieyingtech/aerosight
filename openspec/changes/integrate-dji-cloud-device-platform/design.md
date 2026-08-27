## Context

参见 [proposal.md](./proposal.md) 的动机与范围。当前数据库已经存在通用设备、Adapter、命令、直播和算法领域表的早期结构，Go 包也有 DJI 消息转换与回调安全辅助代码，但没有生产 MQTT 会话、完整 Topic 路由、实机命令闭环或媒体网关；Web 的 DJI 连接测试仍明确返回未实现。现有演示器只向标准输出写事件，未经过 Adapter 摄取边界，因此无法驱动页面。

本设计需要同时覆盖 Dock 2/Matrice 3D 系列和 Dock 3/Matrice 4D 系列、局域网与公网部署，并保持 User → Team membership → Project 的租户边界。DJI 产品枚举、命令与直播源存在代际差异，必须通过版本化产品能力矩阵处理，不能散落在 UI 条件分支中。

## Goals / Non-Goals

**Goals:**

- 用同一领域设备模型管理机场、飞行器和未来其他硬件，用拓扑和能力表达差异。
- 形成可在协议演示器和真实 Dock 上复用的 MQTT 摄取、命令、直播和状态恢复路径。
- 让单机局域网演示部署尽可能简单，同时让公网部署具备独立端点、TLS、鉴权和秘密管理边界。
- 将算法系统收敛为通用能力目录，任何具体识别任务只是项目数据和映射配置。
- 先交付可审计的离散命令和航线任务，不把浏览器变成实时飞控器。

**Non-Goals:**

- 不抽象到“所有硬件完全相同”；机场、飞行器、相机仍通过类型化 capability schema 保留领域约束。
- 不在本变更内实现任意连续 DRC/摇杆控制、全套机场维护命令、视频转码器或模型训练。
- 不保证无公网入口、NAT 穿透或证书条件的现场能由平台自动修复网络。
- 不使用算法回调或大模型输出直接触发物理设备动作。

## Decisions

### 1. 无人机和机场都是 `devices`，相机按生命周期选择组件或设备

- `devices` 保存通用身份、`kind`、厂商、型号、显示名称、当前状态和项目归属；建议 `kind` 使用开放注册表代码，例如 `aerial.vehicle`、`dock.station`、`camera.fixed`，而不是数据库封闭 enum。
- `device_relationships` 表达 `gateway-for`、`docked-aircraft`、`mounted-on` 等版本化关系，并以复合约束确保父子同项目。
- `device_components` 表达随主设备一起认领、授权和删除的相机/云台/载荷组件；若一个相机需要独立 Adapter、权限或生命周期，则升级为独立 `device`，关系模型保持不变。
- `device_capabilities` 保存 `capability_code`、schema/version、参数约束、风险等级和可用条件。页面只消费通用状态及 capability descriptors，厂商原始字段保存在扩展 JSON/原始消息对象中。

选择理由：机场与飞行器都有身份、状态、遥测、命令和媒体，使用同一设备层能复用授权、审计和未来 Adapter。把 Dock 建成独立专表会把 DJI 语义扩散到所有查询；把每个相机都建成设备又会产生不必要的认领和授权负担，因此采用设备 + 组件的组合。

### 2. DJI Adapter 是 Go Worker 中的长连接边界

- 每个启用的项目 Adapter 配置由 Worker 租约单实例持有，建立 MQTT 5 会话并订阅该网关序列号下的状态、事件、请求和服务回复 Topic；多 Worker 通过数据库租约避免重复活跃连接。
- 上行首先保存最小原始 envelope/对象存储引用，再调用现有 topology/translation 逻辑转换为统一事件。幂等键由 Adapter 实例、网关序列号、消息 ID/业务 ID 和方法组成，不能只依赖到达顺序。
- 下行只读取已经进入 `dispatchable` 的 `device_commands`，由能力映射表选择 DJI method/topic，生成厂商业务 ID并写入 `command_attempts`；回复按网关、业务 ID 与 method 三元组关联。
- 断线重连使用持久会话或重新订阅后状态同步；无法判断结果的非幂等命令标记 `unknown` 等待人工核查，不能盲目重发。

备选方案是在 Next.js Route Handler 中直接连接 MQTT；长连接、恢复和后台消费不适合请求生命周期，且会模糊同步授权与异步设备面的边界，故不采用。

### 3. 产品能力矩阵隔离 Dock 2/3 差异

- 在代码中维护受测试保护的 `vendor=dji + domain + type + subtype + firmware/api range` 产品矩阵，输出统一设备类别、拓扑角色、状态映射、视频源和命令 capability。
- 矩阵按版本注册，并保留设备最后解析版本；新固件或未知枚举先进入 `degraded/read-only`，通过契约夹具验证后再开放控制。
- Dock 2/Matrice 3D/3TD 与 Dock 3/Matrice 4D/4TD 使用独立夹具和映射测试，禁止通过名称前缀猜测能力。

选择理由：单一巨大 switch 容易在新型号上静默套用旧枚举。将差异集中为能力矩阵可以让 UI 和任务层保持厂商无关，也便于以后新增产品。

### 4. 网络使用一个部署 profile、四类外部端点

`device_adapters.config` 引用一个不可变/版本化连接 profile：

- `mqtt_endpoint`：机场/模拟器可达的 Broker 地址、TLS、client/tenant 认证引用。
- `api_public_base_url`：DJI WebView、设备或 callback 可达的 HTTPS 地址。
- `websocket_public_url`：需要实时协商或 DJI Pilot 接入时的外部地址。
- `media_ingest_base_url` 与 `media_playback_base_url`：分别面向设备推流和用户播放，可不同域名/网段。

`lan` profile 允许 RFC1918 地址，但拒绝 `localhost`/回环作为设备端点；`public` profile 强制可信 TLS、关闭匿名 Broker、限制 Topic ACL，并要求轮换秘密。连接向导执行 DNS、TCP/TLS、Broker 认证、HTTP health 和媒体 publish/play 探测，无法从服务端证明设备侧可达时明确标为“待现场验证”。

备选方案是根据浏览器访问 Host 自动推导所有端点；在 NAT、反向代理和双网卡环境下不可靠，故端点必须显式配置。

### 5. 演示使用 MQTT Broker + MediaMTX，生产接口保持可替换

- 本地/现场演示 compose 增加支持 MQTT 5 和 ACL 的 Broker，以及 MediaMTX。DJI 优先推 RTMP 到 MediaMTX，浏览器优先 WebRTC、回退 HLS；不把媒体字节经过 Web/数据库。
- `live_streams` 保存控制状态和不含永久秘密的 locator；`video_sources` 或设备组件能力保存源 ID、相机位置、质量档位和厂商直播参数。
- Worker 创建随机流 key、调用 DJI `live_start_push` 类服务并轮询/接收回复；媒体网关 webhook/控制 API 确认真实输入后才将会话标记 `live`。停止命令和网关清理由协调器幂等收敛。
- Web 只在项目授权通过后返回短期播放凭据。公网 MediaMTX 位于反向代理后，并限制发布端点与播放端点权限。

选择理由：RTMP 摄取兼容范围较清晰，MediaMTX 能用单一小型组件提供 HLS/WebRTC。直接在 Next.js 转流会造成资源和安全问题；只把厂商 RTMP URL交给浏览器通常不可播放，均不采用。

### 6. 协议演示器复用真实 Adapter，而不是写数据库

- 新演示器扮演 Dock/Pilot 侧客户端，连接同一个 MQTT Broker，发布 Dock 2 或 Dock 3 官方形状的拓扑、状态和事件，并消费平台服务 Topic 返回 ACK/NACK/延迟/超时。
- 视频使用两个可辨识的循环测试源分别代表机场和飞行器，相同地向媒体网关推流。
- 场景文件控制固件、轨迹、掉线、互锁和失败注入。演示器不得直接调用内部数据库或业务 API来伪造成功，这使协议路径成为本地验收测试。

选择理由：现有 stdout simulator 只能测试事件生成，不能证明 Broker 认证、Topic 路由、命令关联或直播链路。协议级模拟器覆盖的风险更接近实机。

### 7. 算法服务使用通用 Provider/Definition/Run 三层模型

- `algorithm_providers` 只保存传输与运行策略：adapter type、base URL、secret ref、health、timeout、并发、allowlist。
- `algorithm_definitions` 与不可变 version 保存名称、能力类别、输入 JSON Schema、参数 JSON Schema、输出映射 DSL、标签/展示元数据；“疑似违建”作为 seed/template 数据而非前端常量。
- `algorithm_runs` 引用定义版本与不可变输入资产版本。Adapter 将 HTTP JSON 等协议转换为 canonical result union：classification、detection、segmentation、keypoints、tracking、OCR、scalar/table、asset/custom。
- UI 用服务端 schema/catalog 渲染列表、参数表单和结果组件；无法标准化的输出保留原始资产引用并明确为 `partial`/`mapping_required`。
- Provider callback 只完成算法运行。任何事件或设备后续动作都必须经规则、任务草案、用户确认和统一命令账本。

备选方案是为每个算法类型写独立页面和字段；短期直观但每接一个算法都要发布前后端，也容易把模型输出误当业务结论，故不采用。

### 8. 数据迁移保持向后兼容和项目隔离

- 使用只增不删迁移：为现有 `devices` 回填开放的 `kind` 与通用厂商信息；将现有 Adapter 外部身份迁入或兼容视图到唯一绑定结构；补充关系/组件/视频源/profile 结构。
- 复用现有命令、直播和算法表时优先增加版本、状态或 schema 字段，不复制第二套平行账本。旧的 `dji` adapter type 仍可读，但只有通过新 profile 验证后才能启用生产连接。
- 所有新表直接含 `project_id`，父子引用使用 `(project_id, id)` 复合外键或等效事务校验。Provider、播放凭据、命令和 Adapter 配置的每次读取都从已授权项目开始。
- 迁移不会自动为历史“违建算法”赋予法律结论；它被转换为普通 definition/category 与现有结果来源。

### 9. 控制安全以离散能力和命令账本为边界

- 第一阶段白名单包含航线任务投递/开始/取消、安全返航，以及设备能力支持的机场调试命令；每项 capability 定义权限、风险级别、状态互锁、二次确认和超时策略。
- 高风险命令在 Web 和 Worker 双重校验；UI 确认不是安全边界。紧急类命令可提高优先级但仍必须入账并等待确认。
- 任意连续 DRC 控制需要独立的低延迟链路、控制权租约、dead-man switch 和更严格现场安全设计，留给后续 Change。

选择理由：最小实机演示首先要可控、可审计、可恢复。直接暴露厂商 Topic 或通用“调用 method”接口会绕过能力限制并扩大事故面。

## Risks / Trade-offs

- [DJI 固件、账号许可或 Cloud API 枚举变化导致实机不兼容] → 锁定并展示兼容矩阵，保存原始消息，以 Dock 2/3 真机契约夹具和只读降级保护控制面。
- [局域网服务端自检不能证明机场到端点的单向可达性] → 提供设备侧上线检查清单和待现场验证状态，以实际 MQTT/RTMP 握手作为最终就绪信号。
- [公网暴露 MQTT 与媒体发布端点扩大攻击面] → TLS、每 Adapter 独立凭据、Topic ACL、随机流 key、速率限制、网络 allowlist 与秘密轮换。
- [同一机场同时只能推有限路数或占用相机资源] → 能力声明并发限制、会话租约和冲突提示；不承诺所有型号同时双路直播。
- [MediaMTX WebRTC 受企业网络或 NAT 限制] → 允许配置 ICE/TURN 并回退 HLS，UI 展示实际协议和延迟。
- [统一设备模型变成最低公分母] → 只统一身份、拓扑、状态和命令 envelope，差异保留在版本化 capability schema 与 Adapter 映射中。
- [动态算法 schema 使 UI 渲染复杂] → 首期限制支持的 JSON Schema 子集和标准结果 union，未知类型采用只读 JSON/资产展示而非猜测语义。
- [协议模拟成功不等于实机飞行安全] → 将模拟、联调和实机验收分级展示；实机控制仍要求现场清单、兼容固件、空域与人工确认。

## Migration Plan

1. 部署只增不删的数据库迁移和通用设备/算法兼容读路径，回填历史记录并运行租户隔离回归。
2. 部署 MQTT Broker、MediaMTX 与网络 profile 校验，默认不开放公网匿名访问；Worker Adapter 功能开关保持关闭。
3. 接入协议演示器，分别完成 Dock 2 和 Dock 3 的发现、状态、命令成功/NACK/超时、机场/无人机双视频源验收。
4. 启用能力驱动设备 UI、直播面板和动态算法目录，移除页面内的具体算法类别常量，以种子数据保留可选示例。
5. 在隔离项目中配置 DJI 开发者信息和局域网 profile，先只读接入 Dock 2/3，再逐项开启直播、机场调试和航线任务。
6. 公网试点启用受信域名、TLS、Topic ACL、媒体发布/播放鉴权和秘密轮换，通过外部网络连通性与安全测试后放量。
7. 完成两代机场的实机证据记录后再默认展示相应控制能力；未经验证的固件保持只读。

回滚时先关闭所有项目的 DJI 下行命令与新直播创建，停止 Adapter 租约并让活动命令进入可审计的暂停/未知状态；随后回滚 Web/Worker。新增表列和历史事件保留，旧版本忽略新结构；媒体网关停止新发布但不立即删除已形成的证据资产。

## Open Questions

- 试点公网域名、证书签发方式、MQTT Broker 发行版和 TURN 服务由部署环境决定；接口与验收行为不依赖具体供应商。
- 实机验收时使用的具体固件版本和 DJI 开发者账号授权将在部署记录中固化，不改变两代产品族和只读降级要求。
