## Purpose

定义厂商无关的设备实时通道契约，使摄像头、无人机、机器人、传感器和其他设备能够依据 Driver 能力提供视频、遥测或传感数据，并在局域网或公网环境中安全订阅。

## ADDED Requirements

### Requirement: 实时通道由设备能力发现
系统 SHALL 允许 Driver 为设备暴露零个或多个具有稳定标识、显示名称、数据类型、schema、单位、质量、协议和可用状态的实时通道；通道 SHALL 归属于一个设备，UI MUST 从有效能力读取通道，不得写死无人机画面、机场画面或特定传感器面板。

#### Scenario: 同时发现无人机与机场视频源
- **GIVEN** DJI 拓扑中的飞行器和机场相机设备均报告视频能力
- **WHEN** Driver 更新设备有效能力
- **THEN** 系统在各相机设备下展示可选择的视频通道及实时可用状态

#### Scenario: 视频源暂不可用
- **GIVEN** 相机被占用、固件不支持或设备离线
- **WHEN** 用户查看直播操作
- **THEN** 系统 SHALL 显示不可用原因并禁止启动按钮，不得创建虚假播放地址

#### Scenario: 传感器实时通道
- **GIVEN** 温度或环境传感器的 DeviceType 绑定 Driver 声明 `stream.sensor.read`
- **WHEN** 设备在线并上报带时间、单位和质量的采样值
- **THEN** 系统在通用实时面板展示最新值和时间序列，不要求新增传感器专用页面

### Requirement: 实时订阅按能力鉴权
系统 SHALL 在创建或恢复实时订阅时校验项目、设备范围和通道对应的 capability action；读取遥测/传感数据与启动或停止主动视频推流 MUST 使用不同权限。

#### Scenario: 只能读取实时数据
- **GIVEN** 用户拥有目标设备 `stream.sensor.read`，但没有 `stream.video.control`
- **WHEN** 用户订阅传感器并尝试启动摄像头推流
- **THEN** 系统允许传感器订阅但 SHALL 拒绝主动推流操作

#### Scenario: 权限被撤销
- **GIVEN** 用户已经建立设备实时订阅
- **WHEN** 其设备范围或 capability action 被撤销
- **THEN** 系统 SHALL 终止或拒绝续订该会话，且不得继续发送受保护数据

### Requirement: 统一直播会话状态机
每次主动视频直播 SHALL 建立项目级会话并记录源设备、视频通道、请求者、厂商推流协议、摄取定位符、播放定位符、状态、时间和失败原因；状态至少覆盖 `requested`、`starting`、`live`、`stopping`、`stopped` 和 `failed`。

#### Scenario: 启动 DJI 直播
- **GIVEN** 摄像头设备的视频通道在线且用户有对应 `stream.video.control` 权限
- **WHEN** 用户启动直播
- **THEN** 系统创建会话、请求 DJI 向受控入口推流，并仅在媒体网关确认收到媒体后标记为 `live`

#### Scenario: 推流命令成功但未收到媒体
- **GIVEN** DJI 返回启动成功但媒体网关在期限内未收到流
- **WHEN** 会话健康评估运行
- **THEN** 系统 SHALL 标记失败或降级、显示排查原因并允许安全停止/重试

### Requirement: 浏览器兼容播放
系统 SHALL 接收受支持的 DJI 推流并向浏览器提供 HLS 或 WebRTC 播放，直播媒体 MUST 不通过 PostgreSQL 或 Next.js 请求体转发；播放失败 SHALL 不影响设备状态和控制命令处理。

#### Scenario: 浏览器播放实时画面
- **GIVEN** 媒体网关已确认直播输入
- **WHEN** 授权用户打开直播面板
- **THEN** 系统返回短期授权的播放信息并展示延迟、分辨率和连接状态

#### Scenario: 浏览器协议不可用
- **GIVEN** 当前浏览器或网络无法使用首选低延迟协议
- **WHEN** 播放器建立连接失败
- **THEN** 系统 SHALL 尝试允许的备用播放协议或明确报告不可用，不得泄露永久推流密钥

### Requirement: 局域网与公网媒体边界
直播配置 SHALL 区分设备可达的摄取地址与用户可达的播放地址；公网入口 MUST 使用 TLS、不可猜测路径或等效鉴权，局域网入口 MUST 使用设备可路由地址并允许部署者限制到受信网段。

#### Scenario: 局域网双地址配置
- **GIVEN** 机场与浏览器位于不同的内网网段或访问入口
- **WHEN** 管理员配置摄取和播放地址
- **THEN** 系统分别验证设备侧和用户侧可达性，不假定二者域名或端口相同

#### Scenario: 未授权播放
- **GIVEN** 用户无目标项目访问权或播放令牌已过期
- **WHEN** 用户请求播放信息或媒体清单
- **THEN** 系统 SHALL 拒绝访问且不得返回摄取地址或秘密

### Requirement: 会话互斥与资源清理
系统 SHALL 根据 Driver/DeviceType 的有效能力限制同时直播或实时订阅数量，并在用户停止、设备离线、租约过期或进程恢复时协调厂商停止命令和网关清理；重复停止 MUST 幂等。

#### Scenario: 达到设备并发限制
- **GIVEN** 视频源已达到厂商声明的并发直播限制
- **WHEN** 用户请求新会话
- **THEN** 系统 SHALL 拒绝或要求停止冲突会话，并展示冲突来源

#### Scenario: Worker 重启后的孤儿会话
- **GIVEN** Worker 在直播期间重启
- **WHEN** 会话协调器恢复
- **THEN** 系统重新核对厂商与媒体网关状态，并收敛为 `live`、`stopped` 或 `failed`，不得无限保留 `starting`

### Requirement: 双视频源模拟验收
协议演示器 SHALL 为模拟机场、飞行器、摄像头和传感器设备提供可辨识的视频与传感实时通道，并经过与实机相同的设备能力、授权、会话和订阅 API 完成验收。

#### Scenario: 切换模拟机场和无人机画面
- **GIVEN** Dock 2 或 Dock 3 演示场景在线
- **WHEN** 用户先后启动飞行器相机与机场相机直播
- **THEN** 页面播放对应的不同测试画面、显示模拟传感数据，并正确关联源设备、会话和审计记录
