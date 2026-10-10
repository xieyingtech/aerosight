# 验证记录

- 2026-10-10：本地隔离 PostGIS 测试通过，覆盖区域保存、幂等、静态设备位置、空项目隔离、成员越权及自交拒绝。快照回归和静态位置优先级验证通过。
- 设备树登记位置读取验证通过。登记不生成 observations，不生成在线状态或 capabilities。
- 前端位置及地图模型相关测试 10/10 通过，pnpm typecheck、pnpm db:check、pnpm build 和 OpenSpec 严格验证通过。迁移与 schema 快照数据库结构对比通过。
- 初次测试环境未指定 PostgreSQL 用户导致连接失败，补全本地测试用户名后重跑通过。
- 新增设备树断言初次使用对象解码，但已有接口实际返回数组；修正测试解码后，重新执行区域、设备登记及位置优先级相关测试通过。
- 完整 pnpm check 尚未运行，change 不归档。线上发布与登记已验证。

## 线上验证

- xieying.tech 项目 1：已登记区域 ID 1–3；固定监控 ID 3–8；巡检车 ID 9–10；环境传感器 ID 11–12。原 DJI 设备 ID 1–2 保留。
- 正式 API 快照返回 3 个区域和 12 台设备，新增 10 台设备均离线、manual-registration 来源，无新增遥测。名称无演示前缀。完整资源清单保存在被忽略的 .build/online-map-registration-results.json。
- 在线浏览器刷新确认平面模式图层计数为巡检区域 3、固定监控 6、巡检车 2、环境传感器 2；切换 3D 并放大红花山后，3 个渐变围栏、类型图标和周边建筑均可见。
- 画面证据：C:/Users/saurl/Documents/Works/26dgx/demo/online-map-3d-20261010.png。
- Node HTTP 建连出现超时，改用 curl 传输完成真实 API 创建；浏览器批量缩放操作超时后重新读取并截图确认最终视图。没有启动录制。
- 新增规范已同步到 openspec/specs/project-map-registration；未执行完整归档检查，保留 change。
