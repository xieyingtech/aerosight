## Context

区域当前在快照中固定为空。设备已具备 config_json 和类型注册表，但没有手动登记入口。前端现有区域围栏与设备图标投影可复用。

## Goals / Non-Goals

Goals：持久化巡检围栏、手动登记设备及静态位置、授权审计、项目隔离、重复请求幂等。
Non-Goals：区域编辑工作台、模拟在线状态、设备协议或控制、飞行限制执行。

## Decisions

- 新增 project_map_regions 增量迁移，使用 PostGIS geometry(MultiPolygon,4326) 保存并校验有效性。通过项目内 registration_key 唯一约束保障幂等。
- 设备使用已有 devices 表；新增 registration_key 列及项目内部分唯一索引，位置写入 config_json。补充固定监控、巡检车及环境传感器的静态类型，无 adapter、无 capabilities，状态 offline。
- POST /api/projects/:id/map-regions 与 POST /api/projects/:id/devices/register 接受名称、登记键及几何/位置；使用项目 device:configure 和 owner/admin 限制，沿用 AuditedWrite。
- 不制造 observations/poses。快照在遥测缺失时读取登记位置，positionSource 为 manual-registration，positionStatus 为 registered，界面显示“登记位置”，不能成为实时控制依据。设备树也读取相同登记位置便于定位。
- 不采用浏览器本地假数据，因为刷新、智能体读取和不同浏览器之间需要一致。

## Risks / Trade-offs

- 静态配置不能代表车辆实时位置，因此保持离线并显示来源。
- 多边形不是官方界线，取场景布置范围，不写入飞行控制系统。
- 新迁移只添加表和索引，不重建或清理生产数据；先完成隔离测试再随常规发布执行。
