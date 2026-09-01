## Purpose

定义厂商无关的统一设备、类型、驱动和能力契约，使无人机、机场、摄像头、机器人、传感器及未来硬件共享身份、拓扑、状态、实时数据、授权和命令基础设施，同时由 Driver 保留协议与能力差异。

## ADDED Requirements

### Requirement: 所有可管理实体使用统一设备身份
系统 SHALL 将无人机、机场、摄像头、机器人、传感器及其他可独立识别实体表示为项目级设备，并以 DeviceType、厂商型号和外部身份描述，不得为某一厂商型号建立独立业务表或以非设备组件旁路设备授权。

#### Scenario: DJI 机场与飞行器入库
- **GIVEN** DJI Connector 发现一套兼容机场及其飞行器
- **WHEN** 连接器按项目纳管策略同步该拓扑
- **THEN** 系统为机场和飞行器分别创建或建议创建稳定设备身份、保存厂商外部身份并建立父子关系

#### Scenario: 后续接入非 DJI 设备
- **GIVEN** 新 Connector 提供符合统一契约的机器人或传感器类型
- **WHEN** 管理员完成认领
- **THEN** 系统 SHALL 通过同一设备目录管理该设备且无需修改 DJI 专用数据结构

#### Scenario: 摄像头与传感器入库
- **GIVEN** Driver 发现挂载在机器人上的摄像头和环境传感器
- **WHEN** 系统认领该拓扑
- **THEN** 摄像头和传感器各自获得 Device 身份、DeviceType 和能力，并通过设备关系挂载到机器人

### Requirement: DeviceType 必须绑定 Driver
每个 Device MUST 引用一个有效的版本化 DeviceType，每个 DeviceType MUST 绑定一个已注册 Driver 及兼容版本范围；Driver SHALL 定义该类型的协议实现、发现规则、基础能力 manifest、状态转换和命令/实时流处理器。

#### Scenario: 通过类型选择驱动
- **GIVEN** 管理员或设备发现流程为设备选择已注册 DeviceType
- **WHEN** 系统启用设备连接
- **THEN** 系统仅使用该 DeviceType 绑定的兼容 Driver 处理设备，不要求页面或业务服务识别厂商协议

#### Scenario: Driver 缺失或版本不兼容
- **GIVEN** DeviceType 绑定的 Driver 未安装、被禁用或版本不满足约束
- **WHEN** 用户查看或操作该类型设备
- **THEN** 系统 SHALL 将设备标记为 `unavailable` 或只读诊断状态，禁止连接和控制且显示明确原因

### Requirement: 所有拓扑节点均为设备
系统 SHALL 仅通过设备之间的父子、挂载、组成、网关或停靠关系表达硬件拓扑；关系 SHALL 保留角色、有效时间和来源，不得用隐藏组件替代需要独立状态、实时数据或权限的设备。

#### Scenario: 查看机场设备树
- **GIVEN** 一个机场关联飞行器、摄像头和传感器设备
- **WHEN** 用户查看设备详情
- **THEN** 系统展示所有设备及父子/挂载关系，并从各设备能力展示视频或实时数据入口

#### Scenario: 拓扑关系跨项目
- **GIVEN** 父设备与候选子设备属于不同项目
- **WHEN** 用户尝试创建关系
- **THEN** 系统 SHALL 拒绝关系且不泄露另一项目的设备信息

### Requirement: 版本化能力驱动操作
每台设备的有效能力 SHALL 从 Driver 基础 manifest 与 DeviceType 能力配置派生，并由固件/设备运行时报告和当前状态进行收窄或参数化；运行时数据 MUST 不得增加 Driver 未声明的能力。能力 SHALL 包含 action code、输入/输出 schema、风险等级、实时通道和可用条件，API 与 UI MUST 仅根据有效能力渲染操作和实时视图。

#### Scenario: 能力支持的控制
- **GIVEN** 在线设备声明某项命令能力且参数满足约束
- **WHEN** 有权限用户打开设备操作区
- **THEN** 系统显示该操作及当前可用条件，并通过统一命令入口提交

#### Scenario: 型号不支持某项能力
- **GIVEN** 设备未声明目标能力或当前状态使能力不可用
- **WHEN** 用户尝试通过 API 或旧页面提交命令
- **THEN** 系统 SHALL 拒绝命令并返回可解释原因，不得向 Connector 投递

#### Scenario: 运行时收窄基础能力
- **GIVEN** Driver 和 DeviceType 声明视频直播能力，但设备固件或当前链路报告该能力不可用
- **WHEN** 系统计算设备有效能力
- **THEN** 系统保留能力定义但标记当前不可用及原因，不得显示可执行的启动操作

#### Scenario: 设备上报未知能力
- **GIVEN** 设备上报 Driver manifest 中不存在的 capability code
- **WHEN** Driver 处理能力报告
- **THEN** 系统 SHALL 将其作为未识别厂商扩展隔离，不得自动开放 API、UI 或权限动作

### Requirement: Connector、Driver 与 Device 职责分离
系统 SHALL 将 ConnectorDefinition 作为外部 IoT 平台接入契约，将 ConnectorInstance 作为项目级连接配置和运行实例，将 Driver 作为 DeviceType 的设备语义与能力实现；Connector 负责发现和传输，Driver 负责类型解析、能力、状态、命令与实时通道映射。Connector 本身不得作为 Device 出现在设备树中，但连接器发现的独立网关硬件仍 SHALL 建立 Device 身份。

#### Scenario: 新增 IoT 平台连接器
- **GIVEN** 平台注册一个声明配置 schema、凭据 schema、发现模式、健康检查和兼容 Driver 范围的 ConnectorDefinition
- **WHEN** 项目管理员创建符合该定义的 ConnectorInstance
- **THEN** 系统无需修改 Device 表或厂商专用设备页面即可连接、扫描并产生统一外部身份

#### Scenario: 连接器不能注入未知能力
- **GIVEN** Connector 收到外部平台未被兼容 Driver 声明的动作或属性
- **WHEN** 系统归一化发现结果和消息
- **THEN** 系统 SHALL 隔离该扩展且不得将其直接开放为设备 capability、命令或权限 action

#### Scenario: 类型目录只开放已验收连接器
- **GIVEN** ConnectorDefinition 已注册但其生产接入依赖当前不具备的现场设备验收
- **WHEN** 管理员打开新建连接器类型目录
- **THEN** 系统 SHALL 不把该定义展示为可创建类型
- **AND** 既有实例、历史数据和后端兼容代码保持可读取

### Requirement: Connector 自动发现与增量同步
每个启用的 ConnectorInstance SHALL 根据 ConnectorDefinition 支持的 `push`、`poll`、`subscribe` 或 `manual-import` 模式，在配置的租户、workspace、Topic 或资源范围内自动发现设备和拓扑，并以持久同步游标或等效幂等边界更新 ExternalDeviceIdentity。重复扫描 MUST 不重复创建 Device，且不得扫描 ConnectorInstance 配置范围之外的资源。

#### Scenario: 自动发现多个网关及子设备
- **GIVEN** 一个项目级 DJI Connector 的扫描范围包含多个机场序列号
- **WHEN** 连接器收到机场、飞行器、相机和传感器拓扑
- **THEN** 系统为每个可独立识别硬件幂等更新 ExternalDeviceIdentity、建议 DeviceType 并保留来源拓扑

#### Scenario: 增量同步重复消息
- **GIVEN** 外部 IoT 平台重复返回同一设备或重放旧游标消息
- **WHEN** Connector 执行同步
- **THEN** 系统 SHALL 幂等更新同一外部身份，不得创建重复 Device、绑定或拓扑关系

#### Scenario: 外部设备暂时消失
- **GIVEN** 已纳管设备在一次扫描中未出现或连接器暂时离线
- **WHEN** 同步周期结束
- **THEN** 系统 SHALL 将来源标记为 `missing` 或数据过期并保留 Device、绑定和历史，不得立即删除设备

### Requirement: 发现设备纳管策略
ConnectorInstance SHALL 配置 `automatic`、`review` 或 `observe-only` 纳管策略。只有外部身份唯一、DeviceType 匹配明确、项目范围有效且不存在绑定冲突时，`automatic` 才可创建或更新 Device；`review` SHALL 进入待确认，`observe-only` SHALL 仅更新发现对象而不创建 Device。管理员 SHALL 能将发现对象标记为 managed、ignored 或重新进入待确认。

#### Scenario: 安全自动纳管
- **GIVEN** Connector 使用 `automatic` 策略且发现对象具有唯一身份和确定的兼容 DeviceType
- **WHEN** 自动同步完成
- **THEN** 系统创建稳定 Device、绑定外部身份、同步拓扑和能力，并记录自动纳管审计事件

#### Scenario: 未知型号等待确认
- **GIVEN** Connector 发现未知型号或存在多个可能 DeviceType
- **WHEN** 自动同步完成
- **THEN** 系统 SHALL 将对象置为 `discovered` 待确认状态、保持只读诊断且不得开放控制能力

#### Scenario: 观察模式不创建资产
- **GIVEN** ConnectorInstance 使用 `observe-only` 策略
- **WHEN** 它发现新的外部设备
- **THEN** 系统仅更新发现目录和拓扑预览，不得自动创建 Device 或授予设备权限

### Requirement: 设备与连接器显式绑定
系统 SHALL 使用显式绑定关联 Device、ConnectorInstance 和 ExternalDeviceIdentity，并记录 `direct`、`gateway` 或 `inherited` 路由角色、优先级和状态。一个 Connector 可绑定多个 Device；同一 Device 可在迁移或主备场景保留多个绑定，但每次下行操作 MUST 解析唯一的有效主路由。更换网络凭据、ConnectorInstance 或兼容 Driver 版本 MUST 不改变设备内部身份和历史关联。

#### Scenario: 更换网络连接配置
- **GIVEN** 已入库设备具有任务、媒体和事件历史
- **WHEN** 管理员将设备重新绑定到兼容的新 Connector 配置
- **THEN** 设备 ID 和历史引用保持不变，新事件使用新的 Connector 版本信息

#### Scenario: 外部身份冲突
- **GIVEN** 同一厂商外部身份已经绑定到项目设备
- **WHEN** Connector 再次尝试将其绑定到另一内部设备
- **THEN** 系统 SHALL 阻止重复绑定并记录可审计冲突

#### Scenario: 主备连接器路由歧义
- **GIVEN** 同一 Device 存在两个健康且优先级相同的下行绑定
- **WHEN** 用户提交主动控制命令
- **THEN** 系统 SHALL 拒绝投递并报告路由冲突，不得同时向两个 Connector 下发命令

### Requirement: 设备页面呈现连接器发现目录
设备页面 SHALL 汇总项目所有 Connector 的发现结果，并区分已纳管设备、待确认对象、身份冲突、已忽略对象和来源缺失设备；管理员 SHALL 能从设备页面触发授权扫描、确认纳管、忽略或重新匹配类型。连接器端点、凭据、扫描范围和同步日志 MUST 在独立连接器页面管理，不得混入单台设备详情表单。

#### Scenario: 从设备页面处理发现对象
- **GIVEN** 项目连接器发现一个等待确认的摄像头
- **WHEN** 管理员在设备页面选择兼容 DeviceType 并确认纳管
- **THEN** 系统创建或绑定稳定 Device 并在设备树中按发现拓扑显示该摄像头

#### Scenario: 普通成员查看设备目录
- **GIVEN** 项目成员拥有设备查看权限但没有连接器配置权限
- **WHEN** 该成员打开设备页面
- **THEN** 系统允许查看其授权范围内的已纳管设备，但隐藏扫描、纳管、忽略、连接器端点和诊断秘密操作

### Requirement: 统一设备状态投影
系统 SHALL 从 Connector 连接、最近心跳和设备事件派生 `online`、`degraded`、`offline` 或 `unknown` 状态，并保存状态时间、数据新鲜度、原因及厂商原始状态引用。

#### Scenario: 实时设备状态更新
- **GIVEN** 已认领设备产生有效心跳和状态消息
- **WHEN** Connector 和兼容 Driver 完成身份校验和归一化
- **THEN** 系统更新统一状态投影并向项目订阅者发布变更

#### Scenario: 历史状态过期
- **GIVEN** 设备最后一次状态已超过新鲜度阈值
- **WHEN** 用户查看设备或控制页面
- **THEN** 系统 SHALL 显示离线或降级及最后更新时间，不得把缓存值展示为实时事实

### Requirement: RBAC 按设备能力授权
系统 SHALL 在团队角色默认权限之上，按项目、设备范围和 capability action 执行授权；设备范围 SHALL 至少支持项目全部设备、指定 DeviceType 和指定 Device。查看状态、订阅实时数据、启动直播、修改配置和主动控制 MUST 使用可区分的 action。

#### Scenario: 只读传感器操作者
- **GIVEN** 用户被授权读取项目传感器状态和 `stream.sensor.read`，但未获任何控制 action
- **WHEN** 用户查看传感器实时数据并尝试修改采样配置
- **THEN** 系统允许实时订阅但 SHALL 拒绝配置命令

#### Scenario: 仅控制指定无人机
- **GIVEN** 用户拥有某一无人机的 `mission.execute`，但无同类型其他设备的控制授权
- **WHEN** 用户分别尝试操作两台无人机
- **THEN** 系统只允许目标设备命令，并对另一设备返回无权限且不投递 Driver

### Requirement: 统一命令入口与租户授权
所有主动设备操作 MUST 通过项目授权、设备范围、capability action、安全策略和统一命令账本后交给 Driver/Connector；设备页面、任务服务、算法服务和智能体均不得绕过该入口直接调用厂商控制接口。

#### Scenario: 合法设备命令
- **GIVEN** 用户拥有项目操作权限、设备能力可用且安全校验通过
- **WHEN** 用户提交控制请求
- **THEN** 系统创建带请求者、目标、幂等键、截止时间和安全上下文的命令记录后异步投递

#### Scenario: 跨项目或绕过控制
- **GIVEN** 用户无权访问目标项目或内部调用缺少经验证的项目上下文
- **WHEN** 请求尝试控制设备
- **THEN** 系统 SHALL 拒绝请求、不得发布厂商命令并记录审计事件
