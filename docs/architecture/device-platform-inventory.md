# 统一设备平台现状盘点

本文件记录 `integrate-dji-cloud-device-platform` 开始实现时的数据库与服务事实来源，后续迁移和领域代码必须扩展这些账本，不能创建语义重复的第二套状态机。

## 复用的事实来源

| 领域 | 既有事实来源 | 本次处理 |
| --- | --- | --- |
| 设备实例 | `devices` | 保留实例 ID、项目归属、状态和历史引用，新增 `device_type_id` |
| 项目连接实例 | `device_adapters` / `connector_definitions` | 保存版本化类型、项目配置、AES 凭据 envelope、健康状态、纳管策略和 Worker 租约；可创建目录受验收门槛控制 |
| 外部身份 | `device_external_identities` | 继续作为 Adapter 外部 ID 到 Device 的唯一绑定，不新增身份表 |
| 设备路由 | `device_connector_bindings` | direct/gateway/inherited 显式绑定和主备优先级；下行必须解析唯一活动主路由 |
| 连接与遥测 | `device_connections`、`device_telemetry`、`device_latest_telemetry` | 继续作为连接历史、高频事实和最新投影，不建立 DJI 遥测专表 |
| 有效能力 | `device_capabilities` | 扩展为 Driver、DeviceType 和运行状态计算后的有效能力投影，不新增平行能力表 |
| 下行控制 | `device_commands`、`command_attempts` | 继续作为唯一命令账本和投递尝试账本，DJI 服务回复必须关联到这里 |
| 视频会话 | `live_streams` | 继续作为主动视频直播状态机，补充通用设备实时通道引用，不新增 DJI 直播表 |
| 算法服务 | `algorithm_providers`、`algorithm_definitions`、`algorithm_definition_versions` | 保留通用 Provider/Definition/Version 分层，只扩展 schema 和展示元数据 |
| 算法运行 | `algorithm_runs`、`algorithm_run_attempts`、`algorithm_callback_receipts` | 继续作为统一运行、尝试和回调幂等账本，不创建“违建算法运行”表 |
| 粗粒度权限 | `project_permissions` | 保留现有项目权限和角色默认值；新增 capability grant 只承载设备范围例外 |
| 实时通知 | `project_events`、`outbox_events` | 继续承担浏览器游标补发和 Worker 异步处理，媒体字节不进入这两张表 |

## 必须补齐的结构

- Driver 注册表与版本化 DeviceType；每个 Device 必须引用类型，类型必须绑定 Driver。
- 全设备拓扑关系；无人机、机场、机器人、摄像头和传感器都使用 `devices`，不存在组件旁路。
- Driver 声明的实时通道目录，覆盖视频、遥测、传感和事件通道。
- 项目、DeviceType 或 Device 范围的 capability action grant。
- LAN/Public 网络 profile，以及 Adapter 对 profile 的项目内引用。
- 算法定义版本的输出 schema 和动态展示元数据。

## 已知兼容冲突

- `devices.type` 当前是自由文本且被多个查询直接读取。迁移期保留该列作为兼容投影，新代码以 `device_type_id` 为事实来源；所有历史记录先回填到 `legacy.device@1`。
- `device_capabilities` 当前由 Adapter 直接声明。迁移后保留表和唯一业务语义，但增加 Driver/DeviceType 来源与运行时可用性字段；未知运行时 action 不得入表成为有效能力。
- `device_adapters.adapter_type` 当前同时承担协议类型和实现选择。迁移期保留字段，新增 Driver 绑定由 DeviceType 决定，Adapter 仅作为该 Driver 的项目连接实例。
- `live_streams.source_type` 和 `stream_key` 继续兼容旧页面；新会话同时引用 `device_stream_channels`，完成 UI 迁移前不删除旧列。
- 算法 Provider/Definition/Run 已经是通用模型；“疑似违建”仅存在于可选模板和规则数据中，不新增领域专表。

## 基线验证

在新增迁移前执行 `pnpm test:migrations`，验证结果覆盖 PostGIS、空库迁移、既有库升级和重复执行，全部通过。后续迁移测试将在相同入口增加 Driver/DeviceType 回填、项目隔离和跨项目关系拒绝断言。

当前连接器扩展边界见 [连接器开发契约](./connector-development-contract.md)。由于没有 DJI Dock 实机，`dji.cloud-api` 只保留协议实现、fixture 和历史兼容，不进入新建类型目录；当前可创建类型只有只读 `dji.flighthub2`。
