## Purpose

定义厂商无关的统一设备、类型、驱动和能力契约，使无人机、机场、摄像头、机器人、传感器及未来硬件共享身份、拓扑、状态、实时数据、授权和命令基础设施，同时由 Driver 保留协议与能力差异。

## ADDED Requirements

### Requirement: 所有可管理实体使用统一设备身份
系统 SHALL 将无人机、机场、摄像头、机器人、传感器及其他可独立识别实体表示为项目级设备，并以 DeviceType、厂商型号和外部身份描述，不得为某一厂商型号建立独立业务表或以非设备组件旁路设备授权。

#### Scenario: DJI 机场与飞行器入库
- **GIVEN** DJI Adapter 发现一套兼容机场及其飞行器
- **WHEN** 管理员将拓扑认领到目标项目
- **THEN** 系统为机场和飞行器分别创建稳定设备身份、保存厂商外部身份并建立父子关系

#### Scenario: 后续接入非 DJI 设备
- **GIVEN** 新 Adapter 提供符合统一契约的机器人或传感器类型
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
- **THEN** 系统 SHALL 拒绝命令并返回可解释原因，不得向 Adapter 投递

#### Scenario: 运行时收窄基础能力
- **GIVEN** Driver 和 DeviceType 声明视频直播能力，但设备固件或当前链路报告该能力不可用
- **WHEN** 系统计算设备有效能力
- **THEN** 系统保留能力定义但标记当前不可用及原因，不得显示可执行的启动操作

#### Scenario: 设备上报未知能力
- **GIVEN** 设备上报 Driver manifest 中不存在的 capability code
- **WHEN** Driver 处理能力报告
- **THEN** 系统 SHALL 将其作为未识别厂商扩展隔离，不得自动开放 API、UI 或权限动作

### Requirement: Driver 定义与 Adapter 实例分离
系统 SHALL 将 Driver 作为可复用协议实现，将 Adapter 作为项目级连接配置实例；DeviceType 绑定 Driver，Device 通过自身或网关设备关联兼容 Adapter 实例。更换网络凭据、Adapter 实例或兼容 Driver 版本 MUST 不改变设备内部身份和历史关联。

#### Scenario: 更换网络连接配置
- **GIVEN** 已入库设备具有任务、媒体和事件历史
- **WHEN** 管理员将设备重新绑定到兼容的新 Adapter 配置
- **THEN** 设备 ID 和历史引用保持不变，新事件使用新的 Adapter 版本信息

#### Scenario: 外部身份冲突
- **GIVEN** 同一厂商外部身份已经绑定到项目设备
- **WHEN** Adapter 再次尝试将其绑定到另一内部设备
- **THEN** 系统 SHALL 阻止重复绑定并记录可审计冲突

### Requirement: 统一设备状态投影
系统 SHALL 从 Adapter 连接、最近心跳和设备事件派生 `online`、`degraded`、`offline` 或 `unknown` 状态，并保存状态时间、数据新鲜度、原因及厂商原始状态引用。

#### Scenario: 实时设备状态更新
- **GIVEN** 已认领设备产生有效心跳和状态消息
- **WHEN** Adapter 完成身份校验和归一化
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
所有主动设备操作 MUST 通过项目授权、设备范围、capability action、安全策略和统一命令账本后交给 Driver/Adapter；设备页面、任务服务、算法服务和智能体均不得绕过该入口直接调用厂商控制接口。

#### Scenario: 合法设备命令
- **GIVEN** 用户拥有项目操作权限、设备能力可用且安全校验通过
- **WHEN** 用户提交控制请求
- **THEN** 系统创建带请求者、目标、幂等键、截止时间和安全上下文的命令记录后异步投递

#### Scenario: 跨项目或绕过控制
- **GIVEN** 用户无权访问目标项目或内部调用缺少经验证的项目上下文
- **WHEN** 请求尝试控制设备
- **THEN** 系统 SHALL 拒绝请求、不得发布厂商命令并记录审计事件
