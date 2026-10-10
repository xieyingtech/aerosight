## Why

总览已经支持区域围栏和不同设备图标，但没有持久化围栏及手动登记设备位置的入口，无法构成完整的公园巡检空间视图。

## What Changes

- 新增项目巡检区域登记 API，保存名称和 WGS84 Polygon/MultiPolygon，快照读取真实持久化区域。
- 新增手动登记设备 API，使用现有设备类型，保存登记位置。未接入设备保持离线、无控制能力，不生成遥测或运行轨迹。
- 为当前项目配置入口监控、巡检车和巡检区域，名称不添加演示前缀。区域仅用于空间展示，不触发飞行限制或设备控制。
- 不新增设备协议、直播、控制及围栏执行能力。

## Capabilities

### New Capabilities
- `project-map-registration`: 项目级围栏与设备登记位置的持久化、授权和快照投影。

### Modified Capabilities
无。

## Impact

新增独立区域表的增量迁移，保持现有设备和快照兼容。写入限 owner/admin，采用既有 device:configure 权限和审计事务。快照所有读取保持项目隔离和 repeatable-read。登记位置是静态配置，不替代已有实时位置，不提供控制依据。
