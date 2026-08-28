## Purpose

定义 DJI Cloud API 的可部署接入和最小实机控制闭环，兼容 Dock 2/3 及其配套飞行器，并确保厂商协议不会绕过平台统一设备、安全和审计边界。

## ADDED Requirements

### Requirement: 支持两代机场产品族
DJI Driver SHALL 支持 Dock 2 + Matrice 3D/3TD 与 Dock 3 + Matrice 4D/4TD，并为机场、飞行器、可独立识别相机和传感器映射对应 DeviceType、设备拓扑、能力与状态；未知型号 MUST 以受限能力接入而非套用错误型号配置。

#### Scenario: 发现 Dock 2 拓扑
- **GIVEN** 兼容固件的 Dock 2 与 Matrice 3D 或 3TD 连接到项目 Connector
- **WHEN** Connector 收到有效拓扑和状态数据
- **THEN** 系统发现机场、飞行器及可识别相机/传感器设备，绑定 DJI Driver 类型、映射各自能力并允许管理员认领

#### Scenario: 发现 Dock 3 拓扑
- **GIVEN** 兼容固件的 Dock 3 与 Matrice 4D 或 4TD 连接到项目 Connector
- **WHEN** Connector 收到有效拓扑和状态数据
- **THEN** 系统发现机场、飞行器及可识别相机/传感器设备，绑定 DJI Driver 类型、映射该产品族特有能力且不误用 Dock 2 枚举

#### Scenario: 未知或不兼容固件
- **GIVEN** 产品型号或 Cloud API 版本不在兼容矩阵中
- **WHEN** 设备上线
- **THEN** 系统 SHALL 显示 `degraded` 和兼容性原因，允许只读诊断但禁止未验证控制

### Requirement: 局域网与公网连接配置
系统 SHALL 提供显式的 `lan` 和 `public` 部署配置，分别校验设备可达的 MQTT、HTTPS、WebSocket 和直播入口；公网配置 MUST 要求 TLS、身份认证和非开发默认凭据，局域网配置 MUST 拒绝仅本机可达的回环地址作为设备入口。

#### Scenario: 合法局域网部署
- **GIVEN** 管理员配置机场可路由到的内网 Broker、API 与媒体地址
- **WHEN** 执行连接自检
- **THEN** 系统验证地址、端口、认证和服务健康，并给出可用于 DJI 配置的端点摘要

#### Scenario: 合法公网部署
- **GIVEN** 管理员配置具有可信证书和鉴权的公网域名
- **WHEN** 执行连接自检
- **THEN** 系统验证 TLS、服务健康、外部回调和推流入口，并在通过后允许启用 Connector

#### Scenario: 不可路由或不安全地址
- **GIVEN** 配置使用回环地址、无效证书、匿名公网 Broker 或不可达端口
- **WHEN** 管理员测试连接
- **THEN** 系统 SHALL 阻止启用并逐项返回脱敏的失败原因

### Requirement: DJI 会话与上行消息摄取
DJI Connector SHALL 通过项目 ConnectorInstance 建立可恢复的 MQTT 会话并摄取设备拓扑、属性、事件、请求和服务回复；兼容 DJI Driver SHALL 使用设备序列号、消息标识和时间信息进行身份解析、幂等处理与统一事件映射。

#### Scenario: 正常状态摄取
- **GIVEN** 已绑定设备通过认证会话发送有效状态或事件
- **WHEN** Driver 通过 Connector 会话接收消息
- **THEN** 系统保存原始协议引用、更新统一设备状态并发布项目级实时事件

#### Scenario: 重复、乱序或伪造消息
- **GIVEN** 消息已处理、序列明显回退或会话身份与设备绑定不一致
- **WHEN** Driver 处理消息
- **THEN** 系统 SHALL 幂等忽略或隔离消息，不得覆盖更新状态，并记录诊断信息

#### Scenario: MQTT 会话中断后恢复
- **GIVEN** Broker 或网络暂时不可用
- **WHEN** 连接恢复
- **THEN** Connector 自动重连、重新订阅并由 Driver 重新同步当前状态，未确认命令保持可审计的未知或重试状态

### Requirement: DJI 最小飞行控制闭环
系统 SHALL 通过统一任务与命令入口支持兼容设备的航线任务下发、开始、取消和安全返航，并将 DJI 服务回复和进度事件关联到原命令及任务运行；本变更 MUST 不开放未纳入安全策略的任意连续摇杆控制。

#### Scenario: 执行预检通过的航线任务
- **GIVEN** 飞行器、机场和空域/设备预检通过且任务版本已固化
- **WHEN** 有权限用户确认开始任务
- **THEN** 系统按 DJI 协议投递任务、关联执行进度并在 UI 展示已确认状态

#### Scenario: 任务命令超时
- **GIVEN** 任务命令已发布但截止时间前未收到可关联回复
- **WHEN** 超时评估运行
- **THEN** 系统 SHALL 将物理状态标记为未知或暂停，禁止假定成功，并提示人工核查

#### Scenario: 请求安全返航
- **GIVEN** 飞行器在线且声明返航能力
- **WHEN** 授权操作者提交返航并通过安全确认
- **THEN** 系统以高优先级创建命令、等待 DJI 回复并完整记录结果

### Requirement: DJI 机场远程调试控制
系统 SHALL 根据机场实时能力支持最小远程调试命令集，包括调试模式、舱盖、飞行器上下电、充电、声光告警和重启中的设备支持项；高风险命令 MUST 显示现场前置条件并要求二次确认。

#### Scenario: 控制支持的机场能力
- **GIVEN** 机场在线、处于允许状态且声明目标命令能力
- **WHEN** 有权限用户完成风险确认
- **THEN** 系统通过命令账本投递 DJI 服务调用并展示回复、耗时和最终状态

#### Scenario: 状态互锁阻止命令
- **GIVEN** 机场当前状态、飞行器位置或活动任务不允许目标操作
- **WHEN** 用户尝试下发命令
- **THEN** 系统 SHALL 在发布前拒绝并返回触发的互锁规则

### Requirement: DJI 协议级演示器
系统 SHALL 提供不依赖真实 DJI 硬件的协议级演示器，可选择 Dock 2 或 Dock 3 场景，产生拓扑、遥测、命令 ACK/NACK/超时及直播源状态，且使用与实机 Connector 相同的统一接入边界。

#### Scenario: 无硬件完整演示
- **GIVEN** 开发者启动 Dock 3 演示场景
- **WHEN** 用户认领模拟设备并执行控制
- **THEN** 设备目录、命令账本、任务状态和直播入口呈现与实机相同的业务流程，并明确标记为模拟

#### Scenario: 模拟失败路径
- **GIVEN** 场景配置下一条命令为 NACK 或超时
- **WHEN** 用户提交控制
- **THEN** 系统按真实失败状态处理且不得使用测试专用旁路修改业务结果
