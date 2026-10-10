## Context

device_types 与 driver_definitions 已持久化，并由迁移初始化；capability_actions.json 为平台运行时使用的内嵌操作目录。现有页面仅编辑 icon，现有管理 API 受平台管理员校验。

## Goals / Non-Goals

**Goals:** 复用真实目录并可搜索其键、定义与关联，保持历史兼容。

**Non-Goals:** 新建/修改设备类型和驱动，扩大用户权限，执行操作，改变地图图标或计算实例有效能力。

## Decisions

- 保留已有 GET /api/admin/device-types 并扩展其只读字段；新增 /catalog 返回驱动 manifest 和已有操作目录。不新建另一份静态能力清单。
- 由服务端将驱动 manifest 的 capabilities（兼容历史对象与当前数组）及 streams 投影为定义表格行，附来源驱动；不合并同名跨驱动定义，以保留 schema 差异。类型详情展示完整能力 profile。
- 前端共用顶部搜索、表格与点击详情弹窗，类型默认展示分类、厂商/型号、驱动、能力数和状态；其余 tab 按各自字段显示。图标只作类型视觉标识，不再提供编辑框。
- 保留原图标 PATCH API 作兼容，页面不调用；本次无需数据迁移。不引入定义编辑权限或实际设备实例状态。

## Risks / Trade-offs

- [历史 manifest 结构不同] → 兼容对象/数组能力形态并以集成测试覆盖。
- [把声明视为实例已可用] → 页面说明目录与实例在线状态、权限和执行条件不同，状态列只显示定义状态。

## Migration Plan

更新查询和服务端静态前端即可，原接口字段及图标值保留；回滚代码不涉及数据库重建。
